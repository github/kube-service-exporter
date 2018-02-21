package controller

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"

	capi "github.com/hashicorp/consul/api"
)

type ConsulTarget struct {
	client       *capi.Client
	hostIP       string
	kvPrefix     string
	isLeader     bool
	electorStopC chan struct{}
	wg           sync.WaitGroup
	clusterId    string
	mutex        *sync.RWMutex
}

var _ ExportTarget = (*ConsulTarget)(nil)

func NewConsulTarget(cfg *capi.Config, kvPrefix string, clusterId string) (*ConsulTarget, error) {
	hostIP, _, err := net.SplitHostPort(cfg.Address)
	if err != nil {
		return nil, err
	}

	client, err := capi.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &ConsulTarget{
		client:       client,
		hostIP:       hostIP,
		clusterId:    clusterId,
		kvPrefix:     kvPrefix,
		electorStopC: make(chan struct{}),
		wg:           sync.WaitGroup{},
		mutex:        &sync.RWMutex{}}, nil
}

func (t *ConsulTarget) Create(es *ExportedService) (bool, error) {
	asr := t.asrFromExportedService(es)

	hasLeader, err := t.hasLeader()
	if err != nil {
		return false, err
	}

	if !hasLeader {
		return false, fmt.Errorf("No leader found, refusing to create %s", es.Id())
	}

	if err := t.client.Agent().ServiceRegister(asr); err != nil {
		return false, err
	}

	if t.IsLeader() {
		if err := t.writeKV(es); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (t *ConsulTarget) Update(es *ExportedService) (bool, error) {
	return t.Create(es)
}

func (t *ConsulTarget) Delete(es *ExportedService) (bool, error) {
	err := t.client.Agent().ServiceDeregister(es.Id())
	if err != nil {
		return false, err
	}
	return true, nil
}

// Write out metadata to where it belongs using a transaction
func (t *ConsulTarget) writeKV(es *ExportedService) error {
	kvPairs := map[string]string{
		"cluster_name":        es.ClusterId,
		"proxy_protocol":      strconv.FormatBool(es.ProxyProtocol),
		"backend_protocol":    es.BackendProtocol,
		"health_check_path":   es.HealthCheckPath,
		"health_check_port":   strconv.Itoa(int(es.HealthCheckPort)),
		"dns_name":            es.DNSName,
		"load_balancer_class": es.LoadBalancerClass,
	}

	// write out the cluster name as a key
	ops := make([]*capi.KVTxnOp, 0, len(kvPairs))

	for k, v := range kvPairs {
		op := &capi.KVTxnOp{
			Verb:  capi.KVSet,
			Key:   fmt.Sprintf("%s/%s/clusters/%s/%s", t.kvPrefix, es.Id(), es.ClusterId, k),
			Value: []byte(v),
		}
		ops = append(ops, op)
	}

	_, _, _, err := t.client.KV().Txn(ops, nil)
	return err
}

// Manage leader election using Consul Locks
func (t *ConsulTarget) StartElector() error {
	t.wg.Add(1)
	defer t.wg.Done()

	lo := &capi.LockOptions{
		Key:   t.leaderKey(),
		Value: []byte(t.hostIP),
	}

	lock, err := t.client.LockOpts(lo)
	if err != nil {
		return err
	}

	for {
		lockC, err := lock.Lock(t.electorStopC)
		if err != nil {
			log.Printf("Error trying to acquire lock: %+v", err)
			continue
		}

		// we are the leader until lockC is closed or the service stops
		t.setIsLeader(true)

		select {
		case <-lockC:
			t.setIsLeader(false)
			lock.Unlock()
		case <-t.electorStopC:
			t.setIsLeader(false)
			lock.Unlock()
			return nil
		}
	}
}

func (t *ConsulTarget) setIsLeader(val bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.isLeader = val
}

func (t *ConsulTarget) IsLeader() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.isLeader
}
func (t *ConsulTarget) StopElector() {
	close(t.electorStopC)
	t.wg.Wait()
}

func (t *ConsulTarget) leaderKey() string {
	return fmt.Sprintf("%s/leadership/%s-lock", t.kvPrefix, t.clusterId)
}

func (t *ConsulTarget) hasLeader() (bool, error) {
	kvPair, _, err := t.client.KV().Get(t.leaderKey(), &capi.QueryOptions{})
	if err != nil {
		return false, err
	}

	if kvPair == nil {
		return false, nil
	}

	return true, nil
}

func (t *ConsulTarget) asrFromExportedService(es *ExportedService) *capi.AgentServiceRegistration {
	return &capi.AgentServiceRegistration{
		ID:      es.Id(),
		Name:    es.Id(),
		Tags:    []string{es.ClusterId},
		Port:    int(es.Port),
		Address: t.hostIP,
		Check: &capi.AgentServiceCheck{
			Name:     "NodePort",
			TCP:      fmt.Sprintf("%s:%d", t.hostIP, es.Port),
			Interval: "10s",
		},
	}
}
