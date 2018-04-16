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
	target     ExportTarget
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

func NewNodeWatcher(config *NodeInformerConfig, namespaces []string, target ExportTarget) *NodeWatcher {
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
				nw.exportNodes()
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				nw.exportNodes()
			},
			DeleteFunc: func(obj interface{}) {
				nw.exportNodes()
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

func (nw *NodeWatcher) exportNodes() {
	var nodes []string

	options := meta_v1.ListOptions{
		LabelSelector: "kubernetes.github.com/role=node",
	}
	nodeList, err := nw.clientset.CoreV1().Nodes().List(options)
	if err != nil {
		log.Println("Error getting node list: ", err)
	}

	for _, node := range nodeList.Items {
		// add some checks for matching node conditions here
		nodes = append(nodes, node.Name)
	}
	if len(nodes) < 1 {
		fmt.Println("Node nodes found")
	}

	if err := nw.target.WriteNodes(nodes); err != nil {
		log.Println("Error writing nodes to target: ", err)
	}
}
