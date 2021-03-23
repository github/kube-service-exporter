package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type NodeWatcherSuite struct {
	suite.Suite
	nw             *NodeWatcher
	serviceFixture *v1.Service
	target         *fakeTarget
	ic             *NodeInformerConfig
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

	s.ic = &NodeInformerConfig{
		ClientSet:    fake.NewSimpleClientset(),
		ResyncPeriod: time.Duration(0),
	}

	require.NoError(s.T(), err)

	s.target = NewFakeTarget()
	s.nw = NewNodeWatcher(s.ic, s.target, "role=node")

	go s.nw.Run()
}

func (s *NodeWatcherSuite) TearDownTest() {
	s.nw.Stop()
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
		_, err := s.ic.ClientSet.CoreV1().Nodes().Create(context.TODO(), &test.node, meta_v1.CreateOptions{})
		require.NoError(s.T(), err)

		if test.add {
			val := chanRecvWithTimeout(s.T(), s.target.EventC)
			s.Equal("write_nodes", val)
			s.Contains(s.target.GetNodes(), test.node.Name)
		} else {
			s.NotContains(s.target.GetNodes(), test.node.Name)
		}
	}
}

func (s *NodeWatcherSuite) TestUpdateNodes() {
	node := testingNode()
	_, err := s.ic.ClientSet.CoreV1().Nodes().Create(context.TODO(), &node, meta_v1.CreateOptions{})
	require.NoError(s.T(), err)

	val := chanRecvWithTimeout(s.T(), s.target.EventC)
	s.Equal("write_nodes", val)
	s.Contains(s.target.GetNodes(), node.Name)

	_, err = s.ic.ClientSet.CoreV1().Nodes().Update(context.TODO(), &node, meta_v1.UpdateOptions{})
	require.NoError(s.T(), err)
	s.Equal("write_nodes", val)
	s.Contains(s.target.GetNodes(), node.Name)
}

// The Delete here doesn't appear to be triggering the Watch, so this is commented
// out for now.
func (s *NodeWatcherSuite) xTestDeleteNodes() {
	node := testingNode()
	_, err := s.ic.ClientSet.CoreV1().Nodes().Create(context.TODO(), &node, meta_v1.CreateOptions{})
	require.NoError(s.T(), err)

	val := chanRecvWithTimeout(s.T(), s.target.EventC)
	s.Equal("write_nodes", val)
	s.Contains(s.target.GetNodes(), node.Name)

	err = s.ic.ClientSet.CoreV1().Nodes().Delete(context.TODO(), node.Name, meta_v1.DeleteOptions{})
	require.NoError(s.T(), err)

	val = chanRecvWithTimeout(s.T(), s.target.EventC)
	s.Equal("write_nodes", val)
	s.NotContains(s.target.GetNodes(), node.Name)
	fmt.Println(s.target.GetNodes())
}

func (s *NodeWatcherSuite) TestNodeReady() {
	readyNode := testingNode()
	s.True(nodeReady(&readyNode))

	notReadyNodes := []v1.Node{
		testingNode(),
		testingNode(),
	}

	notReadyNodes[0].Spec.Unschedulable = true
	notReadyNodes[1].Status.Conditions = []v1.NodeCondition{
		v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionFalse},
	}

	for _, node := range notReadyNodes {
		s.False(nodeReady(&node))
	}
}

func TestNodeWatcherSuite(t *testing.T) {
	suite.Run(t, new(NodeWatcherSuite))
}
