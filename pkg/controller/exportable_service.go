package controller

import (
	"fmt"

	"k8s.io/api/core/v1"
)

const (
	// ServiceAnnotationLoadBalancerProxyProtocol is the annotation used on the
	// service to signal that the proxy protocol should be enabled.  Set to
	// "*" to indicate that all backends should support Proxy Protocol.
	ServiceAnnotationProxyProtocol = "kube-service-exporter.github.com/load-balancer-proxy-protocol"

	// ServiceAnnotationLoadBalancerBEProtocol is the annotation used on the service
	// to specify the protocol spoken by the backend (pod) behind a listener.
	// Options are `http` or `tcp` for HTTP backends or TCP backends
	ServiceAnnotationLoadBalancerBEProtocol = "kube-service-exporter.github.com/load-balancer-backend-protocol"

	// A path for an HTTP Health check to be delivered to the HealthCheckPort
	ServiceAnnotationLoadBalancerHealthCheckPath = "kube-service-exporter.github.com/load-balancer-health-check-path"

	ServiceAnnotationLoadBalancerFailureDomain = "kube-service-exporter.github.com/load-balancer-failure-domain"

	// If set and set to "false" this will create a separate service
	// *per failure domain*, useful for applications that should not be
	// load balanced across failure domains.
	ServiceAnnotationLoadBalancerServicePerFailureDomain = "kube-service-exporter.github.com/load-balancer-service-per-failure-domain"
)

type ExportedService struct {
	// An Id for the Service, which allows cross-cluster grouped services
	// If two services share the same Id on different clusters, the Service will
	// be namespaced based on the Tag below, so it can be differentiated.
	// Sharing the same Id on the same cluster is probably an error and may
	// result in flapping.
	Id                      string
	Namespace               string
	Name                    string
	FailureDomain           string
	ServicePerFailureDomain bool

	// The Port on which the Service is reachable
	Port int32

	// an optional URI Path for the HealthCheck
	HealthCheckPath string
	HealthCheckPort int32

	// TCP / HTTP
	BackendProtocol string

	// Enable Proxy protocol on the backend
	ProxyProtocol bool
}

// NewExportedServicesFromKubeService returns a slice of ExportedServices, one
// for each v1.Service Port.
func NewExportedServicesFromKubeService(service v1.Service) ([]*ExportedService, error) {
	if service.Spec.Type != v1.ServiceTypeLoadBalancer {
		return nil, fmt.Errorf("%s/%s is not a LoadBalancer Service", service.Namespace, service.Name)
	}

	exportedServices := make([]*ExportedService, 0, len(service.Spec.Ports))
	for i := range service.Spec.Ports {
		es := NewExportedService(service, i)
		exportedServices = append(exportedServices, es)
	}
	return exportedServices, nil
}

// NewExportedService takes in a v1.Service and an index into the
// v1.Service.Ports array and returns an ExportedService.
func NewExportedService(service v1.Service, portIdx int) *ExportedService {
	es := &ExportedService{
		Namespace:               service.Namespace,
		Name:                    service.Name,
		Port:                    service.Spec.Ports[portIdx].NodePort,
		HealthCheckPort:         service.Spec.HealthCheckNodePort,
		ServicePerFailureDomain: true,
		BackendProtocol:         "http",
	}

	if es.HealthCheckPort == 0 {
		es.HealthCheckPort = es.Port
	}

	if service.Annotations != nil {
		if service.Annotations[ServiceAnnotationProxyProtocol] == "*" {
			es.ProxyProtocol = true
		}

		if service.Annotations[ServiceAnnotationLoadBalancerBEProtocol] == "tcp" {
			es.BackendProtocol = "tcp"
		}

		if val, ok := service.Annotations[ServiceAnnotationLoadBalancerHealthCheckPath]; ok {
			es.HealthCheckPath = val
		}

		if val, ok := service.Annotations[ServiceAnnotationLoadBalancerFailureDomain]; ok {
			es.FailureDomain = val
		}

		if service.Annotations[ServiceAnnotationLoadBalancerServicePerFailureDomain] == "false" {
			es.ServicePerFailureDomain = false
		}
	}

	return es
}
