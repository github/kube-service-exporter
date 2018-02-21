package controller

import (
	"fmt"
	"net"
	"strconv"
	"time"

	capi "github.com/hashicorp/consul/api"
)

type ConsulTarget struct {
	client   *capi.Client
	hostIP   string
	kvPrefix string
}

var _ ExportTarget = (*ConsulTarget)(nil)

func NewConsulTarget(cfg *capi.Config, kvPrefix string) (*ConsulTarget, error) {
	hostIP, _, err := net.SplitHostPort(cfg.Address)
	if err != nil {
		return nil, err
	}

	client, err := capi.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &ConsulTarget{
		client:   client,
		hostIP:   hostIP,
		kvPrefix: kvPrefix}, nil
}

func (t *ConsulTarget) Create(es *ExportedService) (bool, error) {
	asr := t.asrFromExportedService(es)

	if err := t.client.Agent().ServiceRegister(asr); err != nil {
		return false, err
	}

	t.withLock(es, func() error {
		return t.writeKV(es)
	})

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

// tries to acquire a lock using Consul leader election, so KV operations are
// only performed by the leader
func (t *ConsulTarget) withLock(es *ExportedService, fn func() error) error {
	lo := &capi.LockOptions{
		Key: fmt.Sprintf("%s/%s/clusters/%s/lock", t.kvPrefix, es.Id(), es.ClusterId),

		// LockWaitTime == 0 defaults to a lock wait of 15s, so specify a short wait
		LockWaitTime: 1 * time.Millisecond,
		LockTryOnce:  true,
	}
	lock, err := t.client.LockOpts(lo)
	if err != nil {
		return err
	}

	if _, err := lock.Lock(make(chan struct{})); err != nil {
		return err
	}
	defer lock.Unlock()

	return fn()
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
