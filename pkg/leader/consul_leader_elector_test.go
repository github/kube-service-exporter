package leader

import (
	"testing"
	"time"

	"github.com/github/kube-service-exporter/pkg/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	KvPrefix  = "kse-test"
	ClusterId = "cluster1"
	ClientId  = "pod1"
)

type ConsulLeaderElectorSuite struct {
	suite.Suite
	consulServer *tests.TestingConsulServer
	elector      *ConsulLeaderElector
}

func WaitForIsLeader(elector LeaderElector) bool {
	for i := 0; i < 10; i++ {
		if elector.IsLeader() {
			return true
		}
		time.Sleep(time.Duration(i*10) * time.Millisecond)
	}
	return false
}

func TestConsulLeaderElectorSuite(t *testing.T) {
	suite.Run(t, new(ConsulLeaderElectorSuite))
}

func (s *ConsulLeaderElectorSuite) SetupTest() {
	s.consulServer = tests.NewTestingConsulServer(s.T())
	s.consulServer.Start()
	elector, err := NewConsulLeaderElector(s.consulServer.Config, KvPrefix, ClusterId, ClientId)
	require.NoError(s.T(), err)
	s.elector = elector
	go s.elector.Run()
}

func (s *ConsulLeaderElectorSuite) TearDownTest() {
	s.elector.Stop()
	s.consulServer.Stop()
}

func (s *ConsulLeaderElectorSuite) TestElection() {
	s.T().Run("Acquires leadership", func(t *testing.T) {
		s.NoError(s.elector.WaitForLeader(5*time.Second, 250*time.Millisecond))
		s.True(s.elector.IsLeader())
	})

	s.T().Run("There can only be one leader", func(t *testing.T) {
		elector2, err := NewConsulLeaderElector(s.consulServer.Config, KvPrefix, ClusterId, "pod2")
		s.NoError(err)
		s.NotNil(elector2)
		go elector2.Run()

		time.Sleep(time.Second)

		ok, err := elector2.HasLeader()
		s.NoError(err)
		s.True(ok, "Second elector sees there is a leader")
		s.False(elector2.IsLeader(), "Second elector is not the leader")
		elector2.Stop()
	})
}

func TestNewElection(t *testing.T) {
	consulServer := tests.NewTestingConsulServer(t)
	consulServer.Start()
	elector1, err := NewConsulLeaderElector(consulServer.Config, KvPrefix, ClusterId, ClientId)
	require.NoError(t, err)
	go elector1.Run()

	assert.NoError(t, elector1.WaitForLeader(5*time.Second, 250*time.Millisecond))
	assert.True(t, elector1.IsLeader())

	elector2, err := NewConsulLeaderElector(consulServer.Config, KvPrefix, ClusterId, "pod2")
	go elector2.Run()
	elector1.Stop()

	assert.True(t, WaitForIsLeader(elector2))
	assert.True(t, elector2.IsLeader())
	elector2.Stop()
	consulServer.Stop()
}
