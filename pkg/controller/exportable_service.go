package controller

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/mitchellh/hashstructure"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
)

const (
	// ServiceAnnotationExported is a boolean that determines whether or not to
	// export the service.  Its value is not stored anywhere in the
	// ExportedService, but it is used in IsExportable()
	ServiceAnnotationExported = "kube-service-exporter.github.com/exported"

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

	// The port the load balancer should listen on for requests routed to this service
	ServiceAnnotationLoadBalancerListenPort = "kube-service-exporter.github.com/load-balancer-listen-port"

	// A path for an HTTP Health check.
	ServiceAnnotationLoadBalancerHealthCheckPath = "kube-service-exporter.github.com/load-balancer-health-check-path"
	// The port for a the Health check. If unset, defaults to the NodePort.
	ServiceAnnotationLoadBalancerHealthCheckPort = "kube-service-exporter.github.com/load-balancer-health-check-port"

	// If set and set to "false" this will create a separate service
	// *per cluster id*, useful for applications that should not be
	// load balanced across multiple clusters.
	ServiceAnnotationLoadBalancerServicePerCluster = "kube-service-exporter.github.com/load-balancer-service-per-cluster"

	ServiceAnnotationLoadBalancerDNSName = "kube-service-exporter.github.com/load-balancer-dns-name"

	// CustomAttrs is like a "junk drawer" - clients can put arbitrary json objects in the annotation, and
	// we'll parse it and make that object available in the consul payload under `.custom_attrs`
	ServiceAnnotationCustomAttrs = "kube-service-exporter.github.com/custom-attrs"
)

type ExportedService struct {
	ClusterId string `json:"ClusterName"`
	Namespace string `json:"-"`
	Name      string `json:"-"`

	// The unique Name for the NodePort. If no name, defaults to the Port
	PortName string `json:"-"`
	// The Port on which the Service is reachable
	Port int32 `json:"port"`

	DNSName           string `json:"dns_name,omitempty"`
	ServicePerCluster bool   `json:"service_per_cluster,omitempty"`

	// an optional URI Path for the HealthCheck
	HealthCheckPath string `json:"health_check_path,omitempty"`

	// HealthCheckPort is a port for the Health Check. Defaults to the NodePort
	HealthCheckPort int32 `json:"health_check_port,omitempty"`

	// TCP / HTTP
	BackendProtocol string `json:"backend_protocol,omitempty"`

	// Enable Proxy protocol on the backend
	ProxyProtocol bool `json:"proxy_protocol,omitempty"`

	// LoadBalancerClass can be used to target the service at a specific load
	// balancer (e.g. "internal", "public"
	LoadBalancerClass string `json:"load_balancer_class,omitempty"`

	// the port the load balancer should listen on
	LoadBalancerListenPort int32 `json:"load_balancer_listen_port,omitempty"`

	CustomAttrs map[string]interface{} `json:"custom_attrs"`

	// Version is a version specifier that can be used to force the Hash function
	// to change and thus rewrite the service metadata. This is useful in cases
	// where the JSON serialization of the object changes, but not the struct
	// itself.
	Version int `json:"-"`
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
		Version:           1,
	}

	if es.PortName == "" {
		// use the container port since it will be consistent across clusters
		// and the NodePort will not.
		es.PortName = strconv.Itoa(int(service.Spec.Ports[portIdx].Port))
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

	if val, ok := service.Annotations[ServiceAnnotationLoadBalancerListenPort]; ok {
		port, err := strconv.ParseInt(val, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("Error setting LoadBalancerListenPort: %v", err)
		}
		es.LoadBalancerListenPort = int32(port)
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

	if val, ok := service.Annotations[ServiceAnnotationLoadBalancerServicePerCluster]; ok {
		parsed, err := strconv.ParseBool(val)
		if err != nil {
			return nil, errors.Wrap(err, "Error setting ServicePerCluster")
		}
		es.ServicePerCluster = parsed
	}

	if val, ok := service.Annotations[ServiceAnnotationCustomAttrs]; ok {
		var customAttrs map[string]interface{}
		err := json.Unmarshal([]byte(val), &customAttrs)
		if err != nil {
			return nil, errors.Wrapf(err, "Error parsing customattrs JSON object")
		}

		es.CustomAttrs = customAttrs
	} else {
		es.CustomAttrs = map[string]interface{}{}
	}

	return es, nil
}

func (es *ExportedService) Hash() (string, error) {
	hash, err := hashstructure.Hash(es, nil)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(hash, 16), nil
}

func IsExportableService(service *v1.Service) bool {
	var exported bool

	if val, ok := service.Annotations[ServiceAnnotationExported]; ok {
		parsed, err := strconv.ParseBool(val)
		if err != nil {
			return false
		}
		exported = parsed
	}
	return exported && service.Spec.Type == v1.ServiceTypeLoadBalancer
}

func (es *ExportedService) MarshalJSON() ([]byte, error) {
	// alias to avoid recursive marshaling
	type ExportedServiceMarshal ExportedService

	hash, err := es.Hash()
	if err != nil {
		return nil, errors.Wrap(err, "Error generating hash during JSON Marshaling")
	}

	data := struct {
		Hash string `json:"hash"`
		ExportedServiceMarshal
	}{
		Hash:                   hash,
		ExportedServiceMarshal: ExportedServiceMarshal(*es),
	}
	return json.Marshal(&data)
}
