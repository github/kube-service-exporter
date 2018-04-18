package controller

import (
	"fmt"
	"log"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type NodeWatcher struct {
	controller cache.Controller
	stopC      chan struct{}
	wg         sync.WaitGroup
	clientset  kubernetes.Interface
}

type NodeInformerConfig struct {
	ClientSet     kubernetes.Interface
	ListerWatcher cache.ListerWatcher
	ResyncPeriod  time.Duration
}

func NewNodeInformerConfig() (*NodeInformerConfig, error) {
	cs, err := NewClientSet()
	if err != nil {
		return nil, err
	}

	lw := cache.NewListWatchFromClient(
		cs.CoreV1().RESTClient(),
		"nodes",
		"",
		fields.Everything())

	return &NodeInformerConfig{
		ClientSet:     cs,
		ListerWatcher: lw,
		ResyncPeriod:  15 * time.Minute,
	}, nil
}

func NewNodeWatcher(config *NodeInformerConfig, namespaces []string, target ExportTarget, nodeSelector string) *NodeWatcher {
	nw := &NodeWatcher{
		stopC:     make(chan struct{}),
		wg:        sync.WaitGroup{},
		clientset: config.ClientSet,
	}

	_, nw.controller = cache.NewInformer(
		config.ListerWatcher,
		&v1.Node{},
		config.ResyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				nw.exportNodes(target, nodeSelector)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				nw.exportNodes(target, nodeSelector)
			},
			DeleteFunc: func(obj interface{}) {
				nw.exportNodes(target, nodeSelector)
			}})

	return nw
}

func (nw *NodeWatcher) Run() {
	nw.controller.Run(nw.stopC)
}

func (nw *NodeWatcher) Stop() {
	close(nw.stopC)
	// wait until the handlers have completed
	nw.wg.Wait()
}

func (nw *NodeWatcher) exportNodes(target ExportTarget, selector string) {
	var nodes []v1.Node
	var options meta_v1.ListOptions

	if len(selector) > 0 {
		options.LabelSelector = selector
	}

	nodeList, err := nw.clientset.CoreV1().Nodes().List(options)
	if err != nil {
		log.Println("Error getting node list: ", err)
	}

	for _, node := range nodeList.Items {
		if nodeReady(node) {
			nodes = append(nodes, node)
		}
	}

	if len(nodes) < 1 {
		fmt.Println("No nodes found")
	}

	if err := target.WriteNodes(nodes); err != nil {
		log.Println("Error writing nodes to target: ", err)
	}
}

func nodeReady(node v1.Node) bool {
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
