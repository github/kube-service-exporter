package leader

import (
	"fmt"
	"log"
	"sync"

	capi "github.com/hashicorp/consul/api"
)

type LeaderElector interface {
	IsLeader() bool
	HasLeader() (bool, error)
}

type ConsulLeaderElector struct {
	client    *capi.Client
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
	kvPair, _, err := le.client.KV().Get(le.leaderKey(), &capi.QueryOptions{})
	if err != nil {
		return false, err
	}

	if kvPair == nil {
		return false, nil
	}

	return true, nil
}

func (le *ConsulLeaderElector) Run() error {
	defer close(le.stoppedC)

	lo := &capi.LockOptions{
		Key:   le.leaderKey(),
		Value: []byte(le.clientId),
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

	le.isLeader = val
}

func (le *ConsulLeaderElector) leaderKey() string {
	return fmt.Sprintf("%s/leadership/%s-leader", le.prefix, le.clusterId)
}
