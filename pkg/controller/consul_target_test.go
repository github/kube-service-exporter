package controller

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/github/kube-service-exporter/pkg/leader"
	"github.com/github/kube-service-exporter/pkg/tests"
	capi "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/core/v1"
)

const (
	KvPrefix  = "kse-test"
	ClusterId = "cluster1"
)

func newDefaultConsulTargetConfig(server *tests.TestingConsulServer) ConsulTargetConfig {
	elector := &fakeElector{isLeader: true, hasLeader: true}
	return ConsulTargetConfig{
		ConsulConfig:    server.Config,
		KvPrefix:        KvPrefix,
		ClusterId:       ClusterId,
		ServicesEnabled: true,
		Elector:         elector}
}

// A fake leader elector
type fakeElector struct {
	isLeader  bool
	hasLeader bool
}

func (fe *fakeElector) IsLeader() bool {
	return fe.isLeader
}

func (fe *fakeElector) HasLeader() (bool, error) {
	return fe.hasLeader, nil
}

// An impatient WaitForLeader that doesn't wait.
func (fe *fakeElector) WaitForLeader(wait, tick time.Duration) error {
	if fe.hasLeader {
		return nil
	}
	return fmt.Errorf("timed out")
}

var _ leader.LeaderElector = (*fakeElector)(nil)

type ConsulTargetSuite struct {
	suite.Suite
	consulServer *tests.TestingConsulServer
}

func TestConsulTargetSuite(t *testing.T) {
	suite.Run(t, new(ConsulTargetSuite))
}

func (s *ConsulTargetSuite) SetupTest() {
	s.consulServer = tests.NewTestingConsulServer(s.T())
	s.consulServer.Start()
}

func (s *ConsulTargetSuite) TearDownTest() {
	s.consulServer.Stop()
}

func (s *ConsulTargetSuite) TestCreate() {
	target, _ := NewConsulTarget(newDefaultConsulTargetConfig(s.consulServer))
	s.T().Run("creates cluster-independent service", func(t *testing.T) {
		es := &ExportedService{
			ClusterId: ClusterId,
			Namespace: "ns1",
			Name:      "name1",
			PortName:  "http",
			Port:      32001}

		ok, err := target.Create(es)
		s.NoError(err)
		s.True(ok)

		services, err := s.consulServer.Client.Agent().Services()
		_, found := services["ns1-name1-http"]
		s.True(found)

		node, _, err := s.consulServer.Client.Catalog().Node(s.consulServer.NodeName, &capi.QueryOptions{})
		require.NoError(s.T(), err)
		_, found = node.Services["ns1-name1-http"]
		s.True(found)
	})

	s.T().Run("creates per-cluster service", func(t *testing.T) {
		es := &ExportedService{
			ClusterId:         ClusterId,
			Namespace:         "ns2",
			Name:              "name2",
			PortName:          "http",
			Port:              32002,
			ServicePerCluster: true,
		}

		ok, err := target.Create(es)
		s.NoError(err)
		s.True(ok)

		services, err := s.consulServer.Client.Agent().Services()
		service, found := services["cluster1-ns2-name2-http"]
		s.True(found)
		if found {
			s.Contains(service.Tags, "cluster1", "service has cluster tag")
		}

		node, _, err := s.consulServer.Client.Catalog().Node(s.consulServer.NodeName, &capi.QueryOptions{})
		s.NoError(err)
		_, found = node.Services["cluster1-ns2-name2-http"]
		s.True(found)
	})

	s.T().Run("Creates per-cluster metadata", func(t *testing.T) {
		es := &ExportedService{
			ClusterId:         ClusterId,
			Namespace:         "ns3",
			Name:              "name3",
			PortName:          "http",
			Port:              32003,
			ServicePerCluster: false,
			LoadBalancerClass: "internal",
			HealthCheckPort:   32303,
		}

		kv := s.consulServer.Client.KV()
		ok, err := target.Create(es)
		s.NoError(err)
		s.True(ok)

		key := fmt.Sprintf("%s/services/%s-%s-%s/clusters/%s", KvPrefix, es.Namespace, es.Name, es.PortName, es.ClusterId)
		pair, _, err := kv.Get(key, &capi.QueryOptions{})
		s.NoErrorf(err, "Expected err for %s to be nil, got %+v", key, err)
		s.NotNilf(pair, "expected KVPair for %s to be not nil", key)

		var meta map[string]interface{}

		err = json.Unmarshal(pair.Value, &meta)
		s.NoError(err)
		s.Equal(meta["load_balancer_class"], "internal")
	})

	s.T().Run("Writes to ServiceKeyTmpl", func(t *testing.T) {
		target, _ := NewConsulTarget(newDefaultConsulTargetConfig(s.consulServer))
		target.serviceKeyTmpl = "services/{{ .LoadBalancerClass }}/{{ id }}"

		es := &ExportedService{
			ClusterId:         ClusterId,
			Namespace:         "ns3",
			Name:              "name3",
			PortName:          "http",
			Port:              32003,
			ServicePerCluster: false,
			LoadBalancerClass: "internal",
			HealthCheckPort:   32303,
		}

		kv := s.consulServer.Client.KV()
		ok, err := target.Create(es)
		s.NoError(err)
		s.True(ok)

		key := fmt.Sprintf("%s/services/%s/%s-%s-%s/clusters/%s", KvPrefix, es.LoadBalancerClass, es.Namespace, es.Name, es.PortName, es.ClusterId)
		pair, _, err := kv.Get(key, &capi.QueryOptions{})
		require.NoErrorf(s.T(), err, "Expected err for %s to be nil, got %+v", key, err)
		require.NotNilf(s.T(), pair, "expected KVPair for %s to be not nil", key)

		var meta map[string]interface{}

		err = json.Unmarshal(pair.Value, &meta)
		s.NoError(err)
		s.Equal(meta["load_balancer_class"], "internal")
	})
}

func (s *ConsulTargetSuite) TestDelete() {
	target, _ := NewConsulTarget(newDefaultConsulTargetConfig(s.consulServer))
	es := &ExportedService{
		ClusterId: ClusterId,
		Namespace: "ns1",
		Name:      "name1",
		PortName:  "http",
		Port:      32001}
	kv := s.consulServer.Client.KV()
	prefix := fmt.Sprintf("%s/services/ns1-name1-http/clusters/%s", KvPrefix, ClusterId)

	ok, err := target.Create(es)
	s.NoError(err)
	s.True(ok)

	services, err := s.consulServer.Client.Agent().Services()
	_, found := services["ns1-name1-http"]
	s.True(found)

	keys, _, err := kv.List(prefix, &capi.QueryOptions{})
	s.NoError(err)
	s.NotEmpty(keys)

	ok, err = target.Delete(es)
	s.NoError(err)
	s.True(ok)

	keys, _, err = kv.List(prefix, &capi.QueryOptions{})
	s.NoError(err)
	s.Empty(keys)

	services, err = s.consulServer.Client.Agent().Services()
	_, found = services["ns1-name1-http"]
	s.False(found)
}

func (s *ConsulTargetSuite) TestShouldUpdateKV() {
	target, _ := NewConsulTarget(newDefaultConsulTargetConfig(s.consulServer))
	es := &ExportedService{
		ClusterId: ClusterId,
		Namespace: "ns1",
		Name:      "name1",
		PortName:  "http",
		Port:      32001}

	ok, err := target.shouldUpdateKV(es)
	s.NoError(err)
	s.True(ok, "Should update KV before first create")

	ok, err = target.Create(es)
	s.NoError(err)
	s.True(ok)

	ok, err = target.shouldUpdateKV(es)
	s.NoError(err)
	s.False(ok, "Should not update KV if same")

	es.Port += 1
	ok, err = target.shouldUpdateKV(es)
	s.NoError(err)
	s.True(ok, "Should update KV after change")
}

func (s *ConsulTargetSuite) TestShouldUpdateService() {
	target, _ := NewConsulTarget(newDefaultConsulTargetConfig(s.consulServer))
	es := &ExportedService{
		ClusterId: ClusterId,
		Namespace: "ns1",
		Name:      "name1",
		PortName:  "http",
		Port:      32001}

	asr := target.asrFromExportedService(es)
	ok, err := target.shouldUpdateService(asr)
	s.NoError(err)
	s.True(ok, "Should update service before first create")

	ok, err = target.Create(es)
	s.NoError(err)
	s.True(ok)

	ok, err = target.shouldUpdateService(asr)
	s.NoError(err)
	s.False(ok, "Should not update service if AgentServiceRegistration same")

	es.Port += 1
	asr = target.asrFromExportedService(es)
	ok, err = target.shouldUpdateService(asr)
	s.NoError(err)
	s.True(ok, "Should update Service after change")
}

func (s *ConsulTargetSuite) TestShouldWriteNodes() {
	target, _ := NewConsulTarget(newDefaultConsulTargetConfig(s.consulServer))
	var exportedNodes []ExportedNode
	node := testingNode()
	target.WriteNodes([]*v1.Node{&node})

	key := fmt.Sprintf("%s/nodes/%s", KvPrefix, ClusterId)
	pair, meta, err := s.consulServer.Client.KV().Get(key, nil)
	require.NoError(s.T(), err)
	s.NotNilf(pair, "%s should exist")
	require.NoError(s.T(), json.Unmarshal(pair.Value, &exportedNodes))
	s.Len(exportedNodes, 1, "should write 1 node")
	lastIndex := meta.LastIndex

	target.WriteNodes([]*v1.Node{&node})
	_, meta, _ = s.consulServer.Client.KV().Get(key, nil)
	s.Equal(lastIndex, meta.LastIndex, "Should not write duplicate data")
}

func (s *ConsulTargetSuite) TestShouldNotUpdateService() {
	elector := &fakeElector{isLeader: true, hasLeader: true}
	target, _ := NewConsulTarget(ConsulTargetConfig{
		ConsulConfig:    s.consulServer.Config,
		KvPrefix:        KvPrefix,
		ClusterId:       ClusterId,
		ServicesEnabled: false,
		Elector:         elector})

	es := &ExportedService{
		ClusterId: ClusterId,
		Namespace: "ns1",
		Name:      "name1",
		PortName:  "http",
		Port:      32001}

	asr := target.asrFromExportedService(es)
	ok, err := target.shouldUpdateService(asr)
	s.NoError(err)
	s.False(ok, "Services should not get updated when ServicesEnabled is false")
}
