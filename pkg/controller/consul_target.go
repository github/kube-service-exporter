package controller

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/pkg/errors"

	"github.com/github/kube-service-exporter/pkg/leader"
	capi "github.com/hashicorp/consul/api"
)

type ConsulTarget struct {
	client    *capi.Client
	elector   leader.LeaderElector
	hostIP    string
	kvPrefix  string
	wg        sync.WaitGroup
	clusterId string
}

var _ ExportTarget = (*ConsulTarget)(nil)

func NewConsulTarget(cfg *capi.Config, kvPrefix string, clusterId string, elector leader.LeaderElector) (*ConsulTarget, error) {
	hostIP, _, err := net.SplitHostPort(cfg.Address)
	if err != nil {
		return nil, err
	}

	client, err := capi.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &ConsulTarget{
		client:    client,
		elector:   elector,
		hostIP:    hostIP,
		clusterId: clusterId,
		kvPrefix:  kvPrefix}, nil
}

func (t *ConsulTarget) Create(es *ExportedService) (bool, error) {
	asr := t.asrFromExportedService(es)

	hasLeader, err := t.elector.HasLeader()
	if err != nil {
		return false, errors.Wrap(err, "Unable to determine leader")
	}

	if !hasLeader {
		return false, errors.Wrapf(err, "No leader found, refusing to create %s", es.Id())
	}

	if err := t.client.Agent().ServiceRegister(asr); err != nil {
		return false, err
	}

	if t.elector.IsLeader() {
		log.Printf("[LEADER] Writing KV metadata for %s", es.Id())
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

	if t.elector.IsLeader() {
		log.Printf("[LEADER] Deleting KV metadata for %s", es.Id())
		t.client.KV().DeleteTree(t.metadataPrefix(es), &capi.WriteOptions{})
	}

	return true, nil
}

func (t *ConsulTarget) metadataPrefix(es *ExportedService) string {
	return fmt.Sprintf("%s/services/%s/clusters/%s", t.kvPrefix, es.Id(), es.ClusterId)
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

	ops := make([]*capi.KVTxnOp, 0, len(kvPairs))

	// write out the cluster-level key explicitly so it can be enumerated with
	// a consul-template ls operation
	ops = append(ops, &capi.KVTxnOp{Verb: capi.KVSet, Key: t.metadataPrefix(es)})
	for k, v := range kvPairs {
		op := &capi.KVTxnOp{
			Verb:  capi.KVSet,
			Key:   fmt.Sprintf("%s/%s", t.metadataPrefix(es), k),
			Value: []byte(v),
		}
		ops = append(ops, op)
	}

	_, _, _, err := t.client.KV().Txn(ops, nil)
	return err
}

func (t *ConsulTarget) asrFromExportedService(es *ExportedService) *capi.AgentServiceRegistration {
	return &capi.AgentServiceRegistration{
		ID:      es.Id(),
		Name:    es.Id(),
		Tags:    []string{es.ClusterId, "kube-service-exporter"},
		Port:    int(es.Port),
		Address: t.hostIP,
		Check: &capi.AgentServiceCheck{
			Name:     "NodePort",
			TCP:      fmt.Sprintf("%s:%d", t.hostIP, es.Port),
			Interval: "10s",
		},
	}
}
