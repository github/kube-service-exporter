package controller

import (
	"log"
	"sync"
	"time"

	"github.com/github/kube-service-exporter/pkg/stats"
	"github.com/github/kube-service-exporter/pkg/util"

	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type ServiceWatcher struct {
	controller cache.Controller
	stopC      chan struct{}
	wg         sync.WaitGroup
	clusterId  string
}

type InformerConfig struct {
	ClientSet     kubernetes.Interface
	ListerWatcher cache.ListerWatcher
	ResyncPeriod  time.Duration
}

func NewClientSet() (kubernetes.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func NewInformerConfig() (*InformerConfig, error) {
	cs, err := NewClientSet()
	if err != nil {
		return nil, err
	}

	lw := cache.NewListWatchFromClient(
		cs.CoreV1().RESTClient(),
		"services",
		meta_v1.NamespaceAll,
		fields.Everything())

	return &InformerConfig{
		ClientSet:     cs,
		ListerWatcher: lw,
		ResyncPeriod:  15 * time.Minute,
	}, nil
}

func NewServiceWatcher(config *InformerConfig, namespaces []string, clusterId string, target ExportTarget) *ServiceWatcher {
	sw := &ServiceWatcher{
		stopC:     make(chan struct{}),
		wg:        sync.WaitGroup{},
		clusterId: clusterId,
	}

	_, sw.controller = cache.NewInformer(
		config.ListerWatcher,
		&v1.Service{},
		config.ResyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				svc, ok := obj.(*v1.Service)
				if !ok {
					log.Println("AddFunc received invalid Service: ", svc)
					return
				}

				// ignore namespaces we don't care about
				if len(namespaces) > 0 && !util.StringInSlice(svc.Namespace, namespaces) {
					return
				}

				sw.addService(obj.(*v1.Service), target)
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

				// ignore namespaces we don't care about
				if len(namespaces) > 0 && !util.StringInSlice(oldSvc.Namespace, namespaces) {
					return
				}
				sw.updateService(oldSvc, newSvc, target)
			},
			DeleteFunc: func(obj interface{}) {
				svc, ok := obj.(*v1.Service)
				if !ok {
					log.Println("DeleteFunc received invalid Service: ", svc)
					return
				}

				// ignore namespaces we don't care about
				if len(namespaces) > 0 && !util.StringInSlice(svc.Namespace, namespaces) {
					return
				}

				sw.deleteService(svc, target)
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

func (sw *ServiceWatcher) addService(service *v1.Service, target ExportTarget) {
	start := time.Now()
	defer sw.wg.Done()
	sw.wg.Add(1)
	tags := []string{"handler:add"}
	stats.Client().Incr("kubernetes.service_handler", tags, 1)
	defer stats.Client().Timing("kubernetes.service_handler.time", time.Since(start), tags, 1)

	if !IsExportableService(service) {
		return
	}
	exportedServices, _ := NewExportedServicesFromKubeService(service, sw.clusterId)

	for _, es := range exportedServices {
		log.Printf("Add service %s", es.Id())
		_, err := target.Create(es)
		stats.IncrSuccessOrFail(err, "target.service", []string{"handler:create", "service:" + es.Id()})
		if err != nil {
			log.Printf("Error adding %+v", es)
		}
	}
}

func (sw *ServiceWatcher) updateService(oldService *v1.Service, newService *v1.Service, target ExportTarget) {
	start := time.Now()
	defer sw.wg.Done()
	sw.wg.Add(1)
	tags := []string{"handler:update"}
	stats.Client().Incr("kubernetes.service_handler", tags, 1)
	defer stats.Client().Timing("kubernetes.service_handler.time", time.Since(start), tags, 1)

	// Delete services that are not exportable (because they aren't LoadBalancer/opt-in)
	if !IsExportableService(newService) {
		// delete the
		sw.deleteService(oldService, target)
	}

	newIds := make(map[string]bool)

	newExportedServices, _ := NewExportedServicesFromKubeService(newService, sw.clusterId)
	for _, es := range newExportedServices {
		newIds[es.Id()] = true
		log.Printf("Update service %s", es.Id())

		_, err := target.Update(es)
		stats.IncrSuccessOrFail(err, "target.service", []string{"handler:update", "service:" + es.Id()})
		if err != nil {
			log.Printf("Error updating %+v", es)
		}
	}

	// delete ExportedServices that are in old, but not new (by Id)
	// This should cover renaming the port name, or a change in other metadata
	// such as ServicePerCluster
	oldExportedServices, _ := NewExportedServicesFromKubeService(oldService, sw.clusterId)
	for _, es := range oldExportedServices {
		if _, ok := newIds[es.Id()]; !ok {
			log.Printf("Delete service %+v due to Id change", es)
			target.Delete(es)
		}
	}
}

func (sw *ServiceWatcher) deleteService(service *v1.Service, target ExportTarget) {
	start := time.Now()
	defer sw.wg.Done()
	sw.wg.Add(1)
	tags := []string{"handler:delete"}
	stats.Client().Incr("kubernetes.service_handler", tags, 1)
	defer stats.Client().Timing("kubernetes.service_handler.time", time.Since(start), tags, 1)

	exportedServices, _ := NewExportedServicesFromKubeService(service, sw.clusterId)
	for _, es := range exportedServices {
		log.Printf("Delete service %s", es.Id())
		_, err := target.Delete(es)
		stats.IncrSuccessOrFail(err, "target.service", []string{"handler:delete", "service:" + es.Id()})
		if err != nil {
			log.Printf("Error deleting %+v", es)
		}
	}
}
