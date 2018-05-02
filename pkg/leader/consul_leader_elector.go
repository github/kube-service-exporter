package leader

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/github/kube-service-exporter/pkg/consul"
	"github.com/github/kube-service-exporter/pkg/stats"
	capi "github.com/hashicorp/consul/api"
	"github.com/pkg/errors"
)

type LeaderElector interface {
	IsLeader() bool
	HasLeader() (bool, error)
	WaitForLeader(wait, tick time.Duration) error
}

type ConsulLeaderElector struct {
	client    *capi.Client
	kv        *consul.InstrumentedKV
	clusterId string
	clientId  string
	isLeader  bool
	mutex     *sync.RWMutex
	prefix    string
	stopC     chan struct{}
	stoppedC  chan struct{}
}

var _ LeaderElector = (*ConsulLeaderElector)(nil)

func NewConsulLeaderElector(cfg *capi.Config, prefix string, clusterId string, clientId string) (*ConsulLeaderElector, error) {
	client, err := capi.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &ConsulLeaderElector{
		client:    client,
		kv:        consul.NewInstrumentedKV(client),
		clientId:  clientId,
		clusterId: clusterId,
		mutex:     &sync.RWMutex{},
		prefix:    prefix,
		stopC:     make(chan struct{}),
		stoppedC:  make(chan struct{}),
	}, nil
}

func (le *ConsulLeaderElector) IsLeader() bool {
	le.mutex.RLock()
	defer le.mutex.RUnlock()

	return le.isLeader
}

func (le *ConsulLeaderElector) HasLeader() (bool, error) {
	qo := capi.QueryOptions{RequireConsistent: true}
	kvPair, _, err := le.kv.Get(le.leaderKey(), &qo, []string{"kv:leader"})
	if err != nil {
		return false, errors.Wrap(err, "In HasLeader")
	}

	if kvPair == nil {
		return false, nil
	}

	if kvPair.Session == "" {
		return false, nil
	}

	return true, nil
}

// WaitForLeader waits for someone to acquire leadership. It returns an error
// if the timer times out.
// wait is how long to wait before timing out
// tick is how often to check
func (le *ConsulLeaderElector) WaitForLeader(wait, tick time.Duration) error {
	waiter := time.NewTimer(wait)
	ticker := time.NewTimer(tick)
	defer waiter.Stop()
	defer ticker.Stop()

	for {
		select {
		case <-waiter.C:
			return fmt.Errorf("No leader found after %s", wait)
		case <-ticker.C:
			if ok, _ := le.HasLeader(); ok {
				return nil
			}
		}
	}
}

func (le *ConsulLeaderElector) Run() error {
	defer close(le.stoppedC)

	lo := &capi.LockOptions{
		Key:          le.leaderKey(),
		Value:        []byte(le.clientId),
		LockWaitTime: 5 * time.Second,
	}

	lock, err := le.client.LockOpts(lo)
	if err != nil {
		return err
	}

	for {
		lockC, err := lock.Lock(le.stopC)
		if err != nil {
			log.Printf("Error trying to acquire lock: %+v", err)
			continue
		}

		// we are the leader until lockC is closed or the service stops
		le.setIsLeader(true)
		log.Println("Elected leader")

		select {
		case <-lockC:
			le.stepDown(lock)
		case <-le.stopC:
			le.stepDown(lock)
			return nil
		}
	}
}

func (le *ConsulLeaderElector) stepDown(lock *capi.Lock) {
	le.setIsLeader(false)
	lock.Unlock()
	log.Println("Leadership relinquished")
}

func (le *ConsulLeaderElector) Stop() {
	close(le.stopC)
	<-le.stoppedC
}

func (le *ConsulLeaderElector) setIsLeader(val bool) {
	le.mutex.Lock()
	defer le.mutex.Unlock()
	tags := []string{"client_id:" + le.clientId}

	le.isLeader = val
	if val {
		stats.Client().Incr("consul.leadership.elected", tags, 1)
	} else {
		stats.Client().Incr("consul.leadership.relinquished", tags, 1)
	}
}

func (le *ConsulLeaderElector) leaderKey() string {
	return fmt.Sprintf("%s/leadership/%s-leader", le.prefix, le.clusterId)
}
