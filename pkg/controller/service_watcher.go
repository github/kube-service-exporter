package controller

import (
	"log"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type ServiceWatcher struct {
	cs         kubernetes.Interface
	controller cache.Controller
	stopC      chan struct{}
	wg         sync.WaitGroup
}

func NewServiceWatcher(clientset kubernetes.Interface, resyncPeriod time.Duration) *ServiceWatcher {
	sw := &ServiceWatcher{
		cs:    clientset,
		stopC: make(chan struct{}),
		wg:    sync.WaitGroup{},
	}

	_, sw.controller = cache.NewInformer(
		cache.NewListWatchFromClient(
			clientset.CoreV1().RESTClient(),
			"services",
			meta_v1.NamespaceAll,
			fields.Everything()),
		&v1.Service{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				svc, ok := obj.(*v1.Service)
				if !ok {
					log.Println("AddFunc received invalid Service: ", svc)
					return
				}

				go sw.addService(obj.(*v1.Service))
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldSvc, ok := oldObj.(*v1.Service)
				if !ok {
					log.Println("UpdateFunc received invalid Service: ", oldObj)
					return
				}

				newSvc, ok := newObj.(*v1.Service)
				if !ok {
					log.Println("UpdateFunc received invalid Service: ", newObj)
					return
				}
				go sw.updateService(oldSvc, newSvc)
			},
			DeleteFunc: func(obj interface{}) {
				svc, ok := obj.(*v1.Service)
				if !ok {
					log.Println("DeleteFunc received invalid Service: ", svc)
					return
				}

				go sw.deleteService(svc)
			}})

	return sw
}

func (sw *ServiceWatcher) Run() {
	sw.controller.Run(sw.stopC)
}

func (sw *ServiceWatcher) Stop() {
	close(sw.stopC)
	// wait until the handlers have completed
	sw.wg.Wait()
}

func (sw *ServiceWatcher) addService(service *v1.Service) {
	defer sw.wg.Done()
	sw.wg.Add(1)

	if service.Spec.Type != v1.ServiceTypeLoadBalancer {
		return
	}
	exportedServices, _ := NewExportedServicesFromKubeService(*service)

	for _, es := range exportedServices {
		log.Printf("Add %+v", es)
	}
}

func (sw *ServiceWatcher) updateService(oldService *v1.Service, newService *v1.Service) {
	defer sw.wg.Done()
	sw.wg.Add(1)

	// Delete services that were changed from LoadBalancers to something else
	if oldService.Spec.Type == v1.ServiceTypeLoadBalancer && newService.Spec.Type != v1.ServiceTypeLoadBalancer {
		sw.deleteService(newService)
	}
	newExportedServices, _ := NewExportedServicesFromKubeService(*newService)
	for _, es := range newExportedServices {
		log.Printf("Update %+v", es)
	}
}

func (sw *ServiceWatcher) deleteService(service *v1.Service) {
	defer sw.wg.Done()
	sw.wg.Add(1)

	exportedServices, _ := NewExportedServicesFromKubeService(*service)
	for _, es := range exportedServices {
		log.Printf("Delete %+v", es)
	}
}
