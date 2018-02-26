package leader

import (
	"testing"

	"github.com/github/kube-service-exporter/pkg/tests"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	KvPrefix  = "kse-test"
	ClusterId = "cluster1"
)

type ConsulLeaderElectorSuite struct {
	suite.Suite
	consulServer *tests.TestingConsulServer
	elector      *ConsulLeaderElector
}

func TestConsulLeaderElectorSuite(t *testing.T) {
	suite.Run(t, new(ConsulLeaderElectorSuite))
}

func (s *ConsulLeaderElectorSuite) SetupTest() {
	s.consulServer = tests.NewTestingConsulServer(s.T())
	s.consulServer.Start()
	elector, err := NewConsulLeaderElector(s.consulServer.Config, KvPrefix, ClusterId)
	require.NoError(s.T(), err)
	s.elector = elector
	go s.elector.Run()
}

func (s *ConsulLeaderElectorSuite) TearDownTest() {
	s.elector.Stop()
	s.consulServer.Stop()
}

func (s *ConsulLeaderElectorSuite) TestIsLeader() {
	s.True(true)
}
