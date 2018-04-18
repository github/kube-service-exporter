package controller

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	fcache "k8s.io/client-go/tools/cache/testing"
)

type NodeWatcherSuite struct {
	suite.Suite
	nw             *NodeWatcher
	serviceFixture *v1.Service
	target         *fakeTarget
	ic             *NodeInformerConfig
	source         *fcache.FakeControllerSource
}

func testingNode() v1.Node {
	return v1.Node{
		ObjectMeta: meta_v1.ObjectMeta{
			Name: "a.example.net",
			Labels: map[string]string{
				"role": "node",
			},
		},
		Spec: v1.NodeSpec{
			Unschedulable: false,
		},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				v1.NodeAddress{
					Address: "192.168.10.10",
					Type:    v1.NodeInternalIP,
				},
			},
			Conditions: []v1.NodeCondition{
				v1.NodeCondition{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}
}

func (s *NodeWatcherSuite) SetupTest() {
	var err error

	// set up a fake ListerWatcher and ClientSet
	s.source = fcache.NewFakeControllerSource()
	s.ic = &NodeInformerConfig{
		ClientSet:     fake.NewSimpleClientset(),
		ListerWatcher: s.source,
		ResyncPeriod:  time.Duration(0),
	}

	require.NoError(s.T(), err)

	s.target = NewFakeTarget()
	s.nw = NewNodeWatcher(s.ic, s.target, "role=node")

	go s.nw.Run()
}

func (s *NodeWatcherSuite) TearDownTest() {
	s.nw.Stop()
}

func (s *NodeWatcherSuite) addNode(node *v1.Node) {
	s.T().Helper()
	// need to add the object to both the clientset and the source since the
	// fake objects don't tie the two together and we do a Nodes().List() inside
	// the watch funcs.
	_, err := s.ic.ClientSet.CoreV1().Nodes().Create(node)
	require.NoError(s.T(), err)
	s.source.Add(node)

}

func (s *NodeWatcherSuite) TestAddNodes() {
	tests := []struct {
		node v1.Node
		add  bool
	}{
		// happy path
		{node: testingNode(), add: true},
		{node: testingNode()},
		{node: testingNode()},
		{node: testingNode()},
	}

	// unscheduleable nodes are not added
	tests[1].node.Spec.Unschedulable = true
	// NotReady nodes are not added
	tests[2].node.Status.Conditions = []v1.NodeCondition{
		v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionFalse},
	}
	// Nodes that don't match the selector are not added
	tests[3].node.Labels = map[string]string{"role": "api"}

	for i, test := range tests {
		test.node.Name = fmt.Sprintf("%d.example.net", i)
		s.addNode(&test.node)

		val := chanRecvWithTimeout(s.T(), s.target.EventC)
		s.Equal("write_nodes", val)
		if test.add {
			s.Contains(s.target.Nodes, test.node.Name)
		} else {
			s.NotContains(s.target.Nodes, test.node.Name)
		}
	}
}

func TestNodeWatcherSuite(t *testing.T) {
	suite.Run(t, new(NodeWatcherSuite))
}
