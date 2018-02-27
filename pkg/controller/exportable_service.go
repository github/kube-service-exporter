package controller

import (
	"fmt"
	"strconv"

	"k8s.io/api/core/v1"
)

const (
	// ServiceAnnotationLoadBalancerProxyProtocol is the annotation used on the
	// service to signal that the proxy protocol should be enabled.  Set to
	// "*" to indicate that all backends should support Proxy Protocol.
	ServiceAnnotationProxyProtocol = "kube-service-exporter.github.com/load-balancer-proxy-protocol"

	// The load balancer class is the target load balancer to apply that the
	// service should be a member of.  Examples might be "internal" or "public"
	ServiceAnnotationLoadBalancerClass = "kube-service-exporter.github.com/load-balancer-class"

	// ServiceAnnotationLoadBalancerBEProtocol is the annotation used on the service
	// to specify the protocol spoken by the backend (pod) behind a listener.
	// Options are `http` or `tcp` for HTTP backends or TCP backends
	ServiceAnnotationLoadBalancerBEProtocol = "kube-service-exporter.github.com/load-balancer-backend-protocol"

	// A path for an HTTP Health check.
	ServiceAnnotationLoadBalancerHealthCheckPath = "kube-service-exporter.github.com/load-balancer-health-check-path"
	// The port for a the Health check. If unset, defaults to the NodePort.
	ServiceAnnotationLoadBalancerHealthCheckPort = "kube-service-exporter.github.com/load-balancer-health-check-port"

	// If set and set to "false" this will create a separate service
	// *per cluster id*, useful for applications that should not be
	// load balanced across multiple clusters.
	ServiceAnnotationLoadBalancerServicePerCluster = "kube-service-exporter.github.com/load-balancer-service-per-cluster"

	ServiceAnnotationLoadBalancerDNSName = "kube-service-exporter.github.com/load-balancer-dns-name"
)

type ExportedService struct {
	ClusterId string
	Namespace string
	Name      string

	// The unique Name for the NodePort. If no name, defaults to the Port
	PortName string
	// The Port on which the Service is reachable
	Port int32

	DNSName           string
	ServicePerCluster bool

	// an optional URI Path for the HealthCheck
	HealthCheckPath string

	// HealthCheckPort is a port for the Health Check. Defaults to the NodePort
	HealthCheckPort int32

	// TCP / HTTP
	BackendProtocol string

	// Enable Proxy protocol on the backend
	ProxyProtocol bool

	// LoadBalancerClass can be used to target the service at a specific load
	// balancer (e.g. "internal", "public"
	LoadBalancerClass string
}

// NewExportedServicesFromKubeService returns a slice of ExportedServices, one
// for each v1.Service Port.
func NewExportedServicesFromKubeService(service *v1.Service, clusterId string) ([]*ExportedService, error) {
	if !IsExportableService(service) {
		return nil, fmt.Errorf("%s/%s is not a LoadBalancer Service", service.Namespace, service.Name)
	}

	exportedServices := make([]*ExportedService, 0, len(service.Spec.Ports))
	for i := range service.Spec.Ports {
		es, err := NewExportedService(service, clusterId, i)
		if err != nil {
			return nil, err
		}
		exportedServices = append(exportedServices, es)
	}
	return exportedServices, nil
}

// An Id for the Service, which allows cross-cluster grouped services
// If two services share the same Id on different clusters, the Service will
// be namespaced based on the Tag below, so it can be differentiated.
func (es *ExportedService) Id() string {
	if es.ServicePerCluster {
		return fmt.Sprintf("%s-%s-%s-%s", es.ClusterId, es.Namespace, es.Name, es.PortName)
	}
	return fmt.Sprintf("%s-%s-%s", es.Namespace, es.Name, es.PortName)
}

// NewExportedService takes in a v1.Service and an index into the
// v1.Service.Ports array and returns an ExportedService.
func NewExportedService(service *v1.Service, clusterId string, portIdx int) (*ExportedService, error) {
	// TODO add some validation to make sure that the clusterId contains only
	//      safe characters for Consul Service names
	if clusterId == "" {
		return nil, fmt.Errorf("No clusterId specified")
	}

	es := &ExportedService{
		Namespace:         service.Namespace,
		Name:              service.Name,
		PortName:          service.Spec.Ports[portIdx].Name,
		Port:              service.Spec.Ports[portIdx].NodePort,
		HealthCheckPort:   service.Spec.Ports[portIdx].NodePort,
		ServicePerCluster: true,
		BackendProtocol:   "http",
		ClusterId:         clusterId,
	}

	if es.PortName == "" {
		es.PortName = strconv.Itoa(int(es.Port))
	}

	if service.Annotations == nil {
		return es, nil
	}

	if val, ok := service.Annotations[ServiceAnnotationLoadBalancerDNSName]; ok {
		es.DNSName = val
	}

	if service.Annotations[ServiceAnnotationProxyProtocol] == "*" {
		es.ProxyProtocol = true
	}

	if val, ok := service.Annotations[ServiceAnnotationLoadBalancerClass]; ok {
		es.LoadBalancerClass = val
	}

	if service.Annotations[ServiceAnnotationLoadBalancerBEProtocol] == "tcp" {
		es.BackendProtocol = "tcp"
	}

	if val, ok := service.Annotations[ServiceAnnotationLoadBalancerHealthCheckPath]; ok {
		es.HealthCheckPath = val
	}

	if val, ok := service.Annotations[ServiceAnnotationLoadBalancerHealthCheckPort]; ok {
		port, err := strconv.ParseInt(val, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("Error setting HealthCheckPort: %v", err)
		}
		es.HealthCheckPort = int32(port)
	}

	if service.Annotations[ServiceAnnotationLoadBalancerServicePerCluster] == "false" {
		es.ServicePerCluster = false
	}

	return es, nil
}

func IsExportableService(service *v1.Service) bool {
	return service.Spec.Type == v1.ServiceTypeLoadBalancer
}
