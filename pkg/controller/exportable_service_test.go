package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ServiceFixture() *v1.Service {
	return &v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "service1",
			Namespace: "default",
			Annotations: map[string]string{
				ServiceAnnotationExported: "true",
			},
		},
		Spec: v1.ServiceSpec{
			Type: "LoadBalancer",
			Ports: []v1.ServicePort{
				v1.ServicePort{
					Name:     "http",
					NodePort: 32123},
				v1.ServicePort{
					Name:     "thing",
					NodePort: 32124},
			},
		},
	}
}

func TestIsExportableService(t *testing.T) {
	t.Run("Service is exportable", func(t *testing.T) {
		svc := ServiceFixture()
		assert.True(t, IsExportableService(svc))
	})

	t.Run("NodePort Service is not exportable", func(t *testing.T) {
		svc := ServiceFixture()
		svc.Spec.Type = "NodePort"
		assert.False(t, IsExportableService(svc))
	})

	t.Run("Service w/out exported annotation is not exportable", func(t *testing.T) {
		svc := ServiceFixture()
		delete(svc.Annotations, ServiceAnnotationExported)
		assert.False(t, IsExportableService(svc))
	})

	t.Run("Service with exported annotation set to false is not exportable", func(t *testing.T) {
		svc := ServiceFixture()
		svc.Annotations[ServiceAnnotationExported] = "false"
		assert.False(t, IsExportableService(svc))
	})

	t.Run("Service with non-bool exported annotation is not exportable", func(t *testing.T) {
		svc := ServiceFixture()
		svc.Annotations[ServiceAnnotationExported] = "pants"
		assert.False(t, IsExportableService(svc))
	})
}

func TestNewExportedService(t *testing.T) {
	t.Run("Bad inputs", func(t *testing.T) {
		svc := ServiceFixture()
		_, err := NewExportedService(svc, "", 0)
		assert.Error(t, err)
	})

	t.Run("Correct defaults", func(t *testing.T) {
		portIdx := 0
		svc := ServiceFixture()
		es, err := NewExportedService(svc, "cluster", portIdx)

		assert.NoError(t, err)
		assert.Equal(t, "cluster", es.ClusterId)
		assert.Equal(t, svc.Namespace, es.Namespace)
		assert.Equal(t, svc.Name, es.Name)
		assert.Equal(t, svc.Spec.Ports[portIdx].NodePort, es.Port)
		assert.Equal(t, svc.Spec.Ports[portIdx].NodePort, es.HealthCheckPort)
		assert.False(t, es.ProxyProtocol)
		assert.Empty(t, es.LoadBalancerClass)
		assert.Equal(t, "http", es.BackendProtocol)
		assert.Empty(t, es.HealthCheckPath)
		assert.True(t, es.ServicePerCluster)
		assert.Empty(t, es.LoadBalancerListenPort)
	})

	t.Run("Overridden defaults", func(t *testing.T) {
		portIdx := 0
		svc := ServiceFixture()
		svc.Annotations[ServiceAnnotationProxyProtocol] = "*"
		svc.Annotations[ServiceAnnotationLoadBalancerClass] = "internal"
		svc.Annotations[ServiceAnnotationLoadBalancerBEProtocol] = "tcp"
		svc.Annotations[ServiceAnnotationLoadBalancerHealthCheckPath] = "/foo/bar"
		svc.Annotations[ServiceAnnotationLoadBalancerHealthCheckPort] = "32001"
		svc.Annotations[ServiceAnnotationLoadBalancerServicePerCluster] = "false"
		svc.Annotations[ServiceAnnotationLoadBalancerListenPort] = "32768"

		es, err := NewExportedService(svc, "cluster", portIdx)
		assert.NoError(t, err)
		assert.True(t, es.ProxyProtocol)
		assert.Equal(t, "tcp", es.BackendProtocol)
		assert.Equal(t, "/foo/bar", es.HealthCheckPath)
		assert.Equal(t, int32(32001), es.HealthCheckPort)
		assert.False(t, es.ServicePerCluster)
		assert.Equal(t, "internal", es.LoadBalancerClass)
		assert.Equal(t, int32(32768), es.LoadBalancerListenPort)
	})

	t.Run("Malformed ServicePerCluster Annotation", func(t *testing.T) {
		svc := ServiceFixture()
		svc.Annotations[ServiceAnnotationLoadBalancerServicePerCluster] = "pants"
		_, err := NewExportedService(svc, "cluster", 0)
		assert.Error(t, err)
	})
}

func TestNewExportedServicesFromKubeServices(t *testing.T) {
	t.Run("LoadBalancer", func(t *testing.T) {
		svc := ServiceFixture()
		exportedServices, err := NewExportedServicesFromKubeService(svc, "cluster")
		assert.NoError(t, err)
		assert.Len(t, exportedServices, len(svc.Spec.Ports))

		for i := range svc.Spec.Ports {
			assert.Equal(t, svc.Name, exportedServices[i].Name)
			assert.Equal(t, svc.Spec.Ports[i].NodePort, exportedServices[i].Port)
		}
	})

	t.Run("Invalid Service Type", func(t *testing.T) {
		svc := ServiceFixture()
		svc.Spec.Type = "NodePort"
		_, err := NewExportedServicesFromKubeService(svc, "cluster")
		assert.Error(t, err)
	})

	t.Run("Invalid clusterId", func(t *testing.T) {
		svc := ServiceFixture()
		_, err := NewExportedServicesFromKubeService(svc, "")
		assert.Error(t, err)
	})
}

func TestId(t *testing.T) {
	t.Run("Id includes cluster and port name", func(t *testing.T) {
		svc := ServiceFixture()
		es, _ := NewExportedService(svc, "cluster", 0)
		assert.Equal(t, "cluster-default-service1-http", es.Id())
	})

	t.Run("Id does not include cluster", func(t *testing.T) {
		svc := ServiceFixture()
		svc.Annotations[ServiceAnnotationLoadBalancerServicePerCluster] = "false"
		es, _ := NewExportedService(svc, "cluster", 0)
		assert.Equal(t, "default-service1-http", es.Id())
	})

	t.Run("Id includes cluster and port number", func(t *testing.T) {
		svc := ServiceFixture()
		svc.Spec.Ports[0].Name = ""
		es, _ := NewExportedService(svc, "cluster", 0)
		assert.Equal(t, "cluster-default-service1-32123", es.Id())
	})
}

func TestHash(t *testing.T) {
	es1, _ := NewExportedService(ServiceFixture(), "cluster", 0)
	hash1, err := es1.Hash()
	assert.Nil(t, err)
	assert.NotEmpty(t, hash1)

	es2, _ := NewExportedService(ServiceFixture(), "cluster", 0)
	hash2, err := es2.Hash()
	assert.Nil(t, err)
	assert.NotEmpty(t, hash2)

	assert.Equal(t, hash1, hash2, "identical ExportedServices should have same hash")

	es2.Port += 1
	hash3, err := es2.Hash()
	assert.Nil(t, err)
	assert.NotEqual(t, hash2, hash3, "different ExportedServices should have different hashes")
}

func TestJSON(t *testing.T) {
	es, _ := NewExportedService(ServiceFixture(), "cluster", 0)
	b, err := json.Marshal(es)
	assert.NoError(t, err)
	expected := `{ "health_check_path": "",
					"health_check_port": 32123,
					"proxy_protocol": false,
					"load_balancer_listen_port": 0,
					"hash": "cdc3e2e77a74ebc8",
					"ClusterName": "cluster",
					"port": 32123,
					"dns_name": "",
					"backend_protocol": "http",
					"load_balancer_class": "" }`

	assert.JSONEq(t, expected, string(b))
}
