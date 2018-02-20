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

type ConsulTargetSuite struct {
	suite.Suite
	consulCmd *exec.Cmd
	consul    *capi.Client
	target    *ConsulTarget
	nodeName  string
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

	os.Setenv("CONSUL_HTTP_ADDR", listener.Addr().String())
	os.Setenv("CONSUL_HTTP_SSL", "false")

	client, err := capi.NewClient(capi.DefaultConfig())
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
	s.target, _ = NewConsulTarget("127.0.0.1", "kse-test")
}

func (s *ConsulTargetSuite) TearDownTest() {
	s.stopConsul()
}

func (s *ConsulTargetSuite) TestCreate() {
	s.T().Run("creates cluster-independent service", func(t *testing.T) {
		es := &ExportedService{
			ClusterId: "cluster1",
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
			ClusterId:         "cluster2",
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
		service, found := services["cluster2-ns2-name2-http"]
		s.True(found)
		s.Contains(service.Tags, "cluster2", "service has cluster tag")

		node, _, err := s.consul.Catalog().Node(s.nodeName, &capi.QueryOptions{})
		s.NoError(err)
		_, found = node.Services["cluster2-ns2-name2-http"]
		s.True(found)
	})

	s.T().Run("Creates per-cluster metadata", func(t *testing.T) {
		es := &ExportedService{
			ClusterId:         "cluster3",
			Namespace:         "ns3",
			Name:              "name3",
			PortName:          "http",
			Port:              32003,
			ServicePerCluster: false,
		}

		kv := s.consul.KV()
		ok, err := s.target.Create(es)
		s.NoError(err)
		s.True(ok)

		pair, _, err := kv.Get("kse-test/ns3-name3-http/clusters/cluster3/cluster_name", &capi.QueryOptions{})
		s.NoError(err)
		s.NotNil(pair)
		s.Equal("cluster3", string(pair.Value))

		pair, _, err = kv.Get("kse-test/ns3-name3-http/clusters/cluster3/proxy_protocol", &capi.QueryOptions{})
		s.NoError(err)
		s.NotNil(pair)
		s.Equal("false", string(pair.Value))
	})
}

func (s *ConsulTargetSuite) TestDelete() {
	es := &ExportedService{
		ClusterId: "cluster1",
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
