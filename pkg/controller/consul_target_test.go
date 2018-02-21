package controller

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	capi "github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	KvPrefix  = "kse-test"
	ClusterId = "cluster1"
)

type ConsulTargetSuite struct {
	suite.Suite
	consulCmd *exec.Cmd
	consul    *capi.Client
	target    *ConsulTarget
	nodeName  string
	consulCfg *capi.Config
}

func TestConsulTargetSuite(t *testing.T) {
	suite.Run(t, new(ConsulTargetSuite))
}

// Start consul in dev mode on a random port for testing against
// Logs will go to stdout/stderr
// Each outer Test* func will get a freshly restarted consul
func (s *ConsulTargetSuite) startConsul() {
	// find a random unused port for Consul to listen on just to reduce the
	// probability that we talk to a production Consul.  This is racey, but
	// should be fine since it's unlikely someone is going to run a consul on
	// our random port between closing this dummy listener and starting aConsul.
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(s.T(), err)
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()

	s.nodeName = fmt.Sprintf("kube-service-exporter-test-%s", port)
	s.consulCmd = exec.Command("consul", "agent", "-dev",
		"-http-port", port, "-bind=127.0.0.1",
		"-node", s.nodeName)
	s.consulCmd.Stdout = os.Stdout
	s.consulCmd.Stderr = os.Stderr
	err = s.consulCmd.Start()
	require.NoError(s.T(), err)

	s.consulCfg = capi.DefaultConfig()
	s.consulCfg.Address = listener.Addr().String()

	client, err := capi.NewClient(s.consulCfg)
	require.NoError(s.T(), err)
	s.consul = client

	startedC := make(chan struct{})
	go func() {
		for {
			_, err := s.consul.KV().Put(&capi.KVPair{Key: "test", Value: []byte("bar")}, nil)
			if err == nil {
				close(startedC)
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// make sure the start doesn't take to long
	timer := time.NewTimer(2 * time.Second)
	select {
	case <-timer.C:
		s.T().Fatal("Took too long to start consul")
	case <-startedC:
	}
}

// Stop consul.  Wait up to 2 seconds before killing it forcefully
func (s *ConsulTargetSuite) stopConsul() {
	s.consulCmd.Process.Signal(syscall.SIGINT)
	stoppedC := make(chan struct{})
	go func() {
		defer close(stoppedC)
		s.consulCmd.Wait()
	}()

	// make sure the stop doesn't take to long
	timer := time.NewTimer(2 * time.Second)
	select {
	case <-timer.C:
		s.T().Fatal("Took too long to stop consul")
	case <-stoppedC:
		s.consulCmd.Process.Kill()
	}
}

func (s *ConsulTargetSuite) SetupTest() {
	s.startConsul()
	s.target, _ = NewConsulTarget(s.consulCfg, KvPrefix, ClusterId)
	go s.target.StartElector()

	// wait until elected
	for i := 0; i < 5; i++ {
		if s.target.IsLeader() {
			break
		}
		time.Sleep(time.Duration(i*100) * time.Millisecond)
	}

	if !s.target.IsLeader() {
		s.T().Fatal("Leader never elected")
	}
}

func (s *ConsulTargetSuite) TearDownTest() {
	s.stopConsul()
	s.target.StopElector()
}

func (s *ConsulTargetSuite) TestCreate() {
	s.T().Run("creates cluster-independent service", func(t *testing.T) {
		es := &ExportedService{
			ClusterId: ClusterId,
			Namespace: "ns1",
			Name:      "name1",
			PortName:  "http",
			Port:      32001}

		ok, err := s.target.Create(es)
		s.NoError(err)
		s.True(ok)

		services, err := s.consul.Agent().Services()
		_, found := services["ns1-name1-http"]
		s.True(found)

		node, _, err := s.consul.Catalog().Node(s.nodeName, &capi.QueryOptions{})
		s.NoError(err)
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

		ok, err := s.target.Create(es)
		s.NoError(err)
		s.True(ok)

		services, err := s.consul.Agent().Services()
		service, found := services["cluster1-ns2-name2-http"]
		s.True(found)
		if found {
			s.Contains(service.Tags, "cluster1", "service has cluster tag")
		}

		node, _, err := s.consul.Catalog().Node(s.nodeName, &capi.QueryOptions{})
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

		kv := s.consul.KV()
		ok, err := s.target.Create(es)
		s.NoError(err)
		s.True(ok)

		expectations := map[string]string{
			"cluster_name":        ClusterId,
			"proxy_protocol":      "false",
			"health_check_port":   "32303",
			"load_balancer_class": "internal",
		}

		for k, v := range expectations {
			key := fmt.Sprintf("%s/%s-%s-%s/clusters/%s/%s", KvPrefix, es.Namespace, es.Name, es.PortName, es.ClusterId, k)
			pair, _, err := kv.Get(key, &capi.QueryOptions{})
			s.NoErrorf(err, "Expected err for %s to be nil, got %+v", key, err)
			s.NotNilf(pair, "expected KVPair for %s to be not nil", key)
			if pair != nil {
				s.Equalf(v, string(pair.Value), k, "expected %s to be %s", v, string(pair.Value))
			}
		}
	})
}

func (s *ConsulTargetSuite) TestDelete() {
	es := &ExportedService{
		ClusterId: ClusterId,
		Namespace: "ns1",
		Name:      "name1",
		PortName:  "http",
		Port:      32001}

	ok, err := s.target.Create(es)
	s.NoError(err)
	s.True(ok)

	services, err := s.consul.Agent().Services()
	_, found := services["ns1-name1-http"]
	s.True(found)

	ok, err = s.target.Delete(es)
	s.NoError(err)
	s.True(ok)

	services, err = s.consul.Agent().Services()
	_, found = services["ns1-name1-http"]
	s.False(found)
}
