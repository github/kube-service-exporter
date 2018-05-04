package controller

import (
	"log"
	"sync"
	"time"

	"github.com/github/kube-service-exporter/pkg/stats"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	informers_v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type NodeWatcher struct {
	informer  informers_v1.NodeInformer
	stopC     chan struct{}
	wg        sync.WaitGroup
	clientset kubernetes.Interface
}

type NodeInformerConfig struct {
	ClientSet    kubernetes.Interface
	ResyncPeriod time.Duration
}

func NewNodeInformerConfig() (*NodeInformerConfig, error) {
	cs, err := NewClientSet()
	if err != nil {
		return nil, err
	}

	return &NodeInformerConfig{
		ClientSet:    cs,
		ResyncPeriod: 15 * time.Minute,
	}, nil
}

func NewNodeWatcher(config *NodeInformerConfig, target ExportTarget, nodeSelector string) *NodeWatcher {
	nw := &NodeWatcher{
		stopC:     make(chan struct{}),
		wg:        sync.WaitGroup{},
		clientset: config.ClientSet,
	}

	sharedInformers := informers.NewSharedInformerFactory(config.ClientSet, config.ResyncPeriod)
	nw.informer = sharedInformers.Core().V1().Nodes()
	nw.informer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				stats.Client().Incr("kubernetes.node_handler", []string{"handler:add"}, 1)
				nw.exportNodes(target, nodeSelector)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				stats.Client().Incr("kubernetes.node_handler", []string{"handler:update"}, 1)
				nw.exportNodes(target, nodeSelector)
			},
			DeleteFunc: func(obj interface{}) {
				stats.Client().Incr("kubernetes.node_handler", []string{"handler:delete"}, 1)
				nw.exportNodes(target, nodeSelector)
			}})

	return nw
}

func (nw *NodeWatcher) Run() error {
	nw.informer.Informer().Run(nw.stopC)
	return nil
}

func (nw *NodeWatcher) Stop() {
	close(nw.stopC)
	// wait until the handlers have completed
	nw.wg.Wait()
}

func (sw *NodeWatcher) String() string {
	return "Kubernetes Node handler"
}

func (nw *NodeWatcher) exportNodes(target ExportTarget, selector string) {
	var readyNodes []*v1.Node

	labelSelector, err := labels.Parse(selector)
	if err != nil {
		log.Println("Error parsing label selector: ", err)
	}

	nodes, err := nw.informer.Lister().List(labelSelector)
	if err != nil {
		log.Println("Error getting node list: ", err)
	}

	for _, node := range nodes {
		if nodeReady(node) {
			readyNodes = append(readyNodes, node)
		}
	}

	if len(nodes) < 1 {
		log.Println("No nodes found")
	}

	if err := target.WriteNodes(nodes); err != nil {
		log.Println("Error writing nodes to target: ", err)
	}
}

func nodeReady(node *v1.Node) bool {
	if node.Spec.Unschedulable {
		return false
	}

	for _, cond := range node.Status.Conditions {
		if cond.Type == v1.NodeReady {
			if cond.Status == v1.ConditionTrue {
				return true
			}
			break
		}
	}

	return false
}
