package controller

import (
	"os"
	"os/exec"
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
}

func TestConsulTargetSuite(t *testing.T) {
	suite.Run(t, new(ConsulTargetSuite))
}

func (s *ConsulTargetSuite) startConsul() {
	s.consulCmd = exec.Command("consul", "agent", "-dev")
	s.consulCmd.Stdout = os.Stdout
	s.consulCmd.Stderr = os.Stderr
	err := s.consulCmd.Start()
	require.NoError(s.T(), err)

	os.Setenv("CONSUL_HTTP_ADDR", "127.0.0.1:8500")
	os.Setenv("CONSUL_HTTP_SSL", "false")

	client, err := capi.NewClient(capi.DefaultConfig())
	require.NoError(s.T(), err)

	startedC := make(chan struct{})
	go func() {
		for {
			_, err := client.KV().Put(&capi.KVPair{Key: "test", Value: []byte("bar")}, nil)
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
}

func (s *ConsulTargetSuite) TearDownTest() {
	s.stopConsul()
}

func (s *ConsulTargetSuite) TestCreate() {
	es := &ExportedService{
		ClusterId: "cluster1",
		Namespace: "default",
		Name:      "foo",
		Port:      32123}

	target, _ := NewConsulTarget()
	ok, err := target.Create(es)
	s.NoError(err)
	s.True(ok)
}
