package controller

import (
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	fcache "k8s.io/client-go/tools/cache/testing"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func chanRecvWithTimeout(t *testing.T, c chan string) string {
	t.Helper()
	select {
	case val := <-c:
		return val
	case <-time.After(100 * time.Millisecond):
		t.Error("Timed out")
	}
	return ""
}

// A fake, in memory, transparent ExportTarget for testing
// Each of the C[r]UD methods sends a blocking event to EventC so the caller
// can detect that the method was called.
type fakeTarget struct {
	Store  []*ExportedService
	EventC chan string
}

func NewFakeTarget() *fakeTarget {
	return &fakeTarget{
		Store:  make([]*ExportedService, 0),
		EventC: make(chan string)}
}

func (t *fakeTarget) Create(es *ExportedService) (bool, error) {
	if _, found := t.find(es); found {
		return false, nil
	}

	t.Store = append(t.Store, es)
	t.EventC <- "create"
	return true, nil
}

func (t *fakeTarget) Update(es *ExportedService) (bool, error) {
	if idx, ok := t.find(es); ok {
		t.Store[idx] = es
		t.EventC <- "update"
		return true, nil
	}
	return false, nil
}

func (t *fakeTarget) Delete(es *ExportedService) (bool, error) {
	if idx, ok := t.find(es); ok {
		t.Store = append(t.Store[:idx], t.Store[idx+1:]...)
		t.EventC <- "delete"
		return true, nil
	}
	return false, nil
}

func (t *fakeTarget) find(es *ExportedService) (int, bool) {
	for i, val := range t.Store {
		if val.Id() == es.Id() {
			return i, true
		}
	}
	return 0, false
}

type ServiceWatcherSuite struct {
	suite.Suite
	sw             *ServiceWatcher
	serviceFixture *v1.Service
	target         *fakeTarget
	ic             *InformerConfig
	source         *fcache.FakeControllerSource
}

func (s *ServiceWatcherSuite) SetupTest() {
	var err error
	ns := &v1.Namespace{ObjectMeta: meta_v1.ObjectMeta{Name: "default"}}

	// set up a fake ListerWatcher and ClientSet
	s.source = fcache.NewFakeControllerSource()
	s.ic = &InformerConfig{
		ClientSet:     fake.NewSimpleClientset(ns),
		ListerWatcher: s.source,
		ResyncPeriod:  time.Duration(0),
	}

	require.NoError(s.T(), err)

	s.ic.ClientSet.CoreV1().Namespaces().Create(ns)
	s.target = NewFakeTarget()

	s.sw = NewServiceWatcher(s.ic, []string{ns.Name}, "cluster", s.target)

	// An example of a "good" service
	s.serviceFixture = &v1.Service{
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

	go s.sw.Run()
}

func (s *ServiceWatcherSuite) TearDownTest() {
	s.sw.Stop()
}

// Helper functions to add/modify/delete a service to the k8s store and wait
// until it has gone thru the fake export target via the ListerWatcher
// Modify sometimes will trigger a delete, so what to expect can be configured
// by passing in "delete" or "update" to expect
func (s *ServiceWatcherSuite) SourceExec(f func(runtime.Object), service *v1.Service, expect string) {
	f(service)
	for i := 0; i < len(service.Spec.Ports); i++ {
		val := chanRecvWithTimeout(s.T(), s.target.EventC)
		s.Equal(expect, val)
	}
}

func (s *ServiceWatcherSuite) TestAdd() {
	s.SourceExec(s.source.Add, s.serviceFixture, "create")
	s.Len(s.target.Store, 2)
}

func (s *ServiceWatcherSuite) TestUpdate() {
	s.SourceExec(s.source.Add, s.serviceFixture, "create")
	s.SourceExec(s.source.Modify, s.serviceFixture, "update")
	s.Len(s.target.Store, 2)
}

func (s *ServiceWatcherSuite) TestDelete() {
	s.SourceExec(s.source.Add, s.serviceFixture, "create")
	s.SourceExec(s.source.Delete, s.serviceFixture, "delete")
	s.Len(s.target.Store, 0)
}

func (s *ServiceWatcherSuite) TestUpdateTriggersDelete() {
	svc := *s.serviceFixture
	s.SourceExec(s.source.Add, &svc, "create")
	s.Len(s.target.Store, 2)
	svc.Spec.Type = "NodePort"
	s.SourceExec(s.source.Modify, &svc, "delete")
	s.Len(s.target.Store, 0)
}

func TestServiceWatcherSuite(t *testing.T) {
	suite.Run(t, new(ServiceWatcherSuite))
}
