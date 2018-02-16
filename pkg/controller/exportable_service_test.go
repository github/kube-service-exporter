package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ServiceFixture() *v1.Service {
	return &v1.Service{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:        "service1",
			Namespace:   "default",
			Annotations: map[string]string{},
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
	svc := ServiceFixture()
	assert.True(t, IsExportableService(svc))

	svc.Spec.Type = "NodePort"
	assert.False(t, IsExportableService(svc))
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
		assert.False(t, es.ProxyProtocol)
		assert.Equal(t, "http", es.BackendProtocol)
		assert.Empty(t, es.HealthCheckPath)
		assert.True(t, es.ServicePerCluster)
	})

	t.Run("Overridden defaults", func(t *testing.T) {
		portIdx := 0
		svc := ServiceFixture()
		svc.Annotations[ServiceAnnotationProxyProtocol] = "*"
		svc.Annotations[ServiceAnnotationLoadBalancerBEProtocol] = "tcp"
		svc.Annotations[ServiceAnnotationLoadBalancerHealthCheckPath] = "/foo/bar"
		svc.Annotations[ServiceAnnotationLoadBalancerServicePerCluster] = "false"

		es, err := NewExportedService(svc, "cluster", portIdx)
		assert.NoError(t, err)
		assert.True(t, es.ProxyProtocol)
		assert.Equal(t, "tcp", es.BackendProtocol)
		assert.Equal(t, "/foo/bar", es.HealthCheckPath)
		assert.False(t, es.ServicePerCluster)
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
	svc := ServiceFixture()
	es, _ := NewExportedService(svc, "cluster", 0)
	assert.Equal(t, "cluster-default-service1-32123", es.Id())
}
