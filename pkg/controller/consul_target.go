package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
	"k8s.io/api/core/v1"

	"github.com/github/kube-service-exporter/pkg/leader"
	"github.com/github/kube-service-exporter/pkg/stats"
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

type ExportedNode struct {
	Name    string
	Address string
}

type exportedNodeList []ExportedNode

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
	var err error
	asr := t.asrFromExportedService(es)

	wait := 15 * time.Second
	if err = t.elector.WaitForLeader(wait, time.Second); err != nil {
		return false, errors.Wrapf(err, "No leader found after %s, refusing to create %s", wait, es.Id())
	}

	updateService, err := t.shouldUpdateService(asr)
	if err != nil {
		return false, errors.Wrap(err, "Error calling shouldUpdateService")
	}

	if updateService {
		log.Printf("Updating Consul Service %s due to registration change", asr.ID)
		if err = t.client.Agent().ServiceRegister(asr); err != nil {
			return false, err
		}
	}

	if t.elector.IsLeader() {
		updateKV, err := t.shouldUpdateKV(es)
		if err != nil {
			return false, errors.Wrap(err, "Error calling shouldUpdateKV")
		}

		if !updateKV {
			return true, nil
		}

		log.Printf("[LEADER] Writing KV metadata for %s", es.Id())
		if err = t.writeKV(es); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (t *ConsulTarget) Update(es *ExportedService) (bool, error) {
	ok, err := t.Create(es)

	return ok, err
}

func (t *ConsulTarget) Delete(es *ExportedService) (bool, error) {
	var err error
	tags := []string{"kv:metadata", "method:delete"}

	err = t.client.Agent().ServiceDeregister(es.Id())
	if err != nil {
		return false, err
	}

	if t.elector.IsLeader() {
		log.Printf("[LEADER] Deleting KV metadata for %s", es.Id())
		stats.WithTiming("consul.kv.time", tags, func() {
			t.client.KV().DeleteTree(t.metadataPrefix(es), &capi.WriteOptions{})
		})
	}

	return true, nil
}

func (t *ConsulTarget) metadataPrefix(es *ExportedService) string {
	return fmt.Sprintf("%s/services/%s/clusters/%s", t.kvPrefix, es.Id(), es.ClusterId)
}

// Write out metadata to where it belongs in Consul using a transaction.  This
// should only ever get called by the current leader
func (t *ConsulTarget) writeKV(es *ExportedService) error {
	var err error
	tags := []string{"kv:metadata", "method:put"}
	defer stats.IncrSuccessOrFail(err, "consul.kv.write", tags)

	esJson, err := json.Marshal(es)
	if err != nil {
		return errors.Wrap(err, "Error marshaling ExportedService JSON")
	}

	kvPair := capi.KVPair{
		Key:   t.metadataPrefix(es),
		Value: esJson,
	}

	stats.WithTiming("consul.kv.time", tags, func() {
		_, err = t.client.KV().Put(&kvPair, nil)
	})

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

// shouldUpdateKV checks if the hash of the ExportedService is the same as the
// hash stored in Consul.  If they differ, KV data is updated.
func (t *ConsulTarget) shouldUpdateKV(es *ExportedService) (bool, error) {
	var kvPair *capi.KVPair
	var err error

	key := t.metadataPrefix(es)
	qo := capi.QueryOptions{RequireConsistent: true}
	tags := []string{"kv:metadata", "method:get"}

	stats.WithTiming("consul.kv.time", tags, func() {
		kvPair, _, err = t.client.KV().Get(key, &qo)
	})
	if err != nil {
		return true, errors.Wrap(err, "Error getting KV hash")
	}

	if kvPair == nil {
		return true, nil
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(kvPair.Value, &meta); err != nil {
		return true, errors.Wrap(err, "Error unmarshaling JSON from Consul")
	}

	consulHash, ok := meta["hash"]
	if !ok {
		return true, nil
	}

	hash, err := es.Hash()
	if err != nil {
		return true, errors.Wrap(err, "Error getting ExportedService Hash in shouldUpdateKV")
	}

	if consulHash == hash {
		return false, nil
	}

	return true, nil
}

// returns true if the active AgentService in Consul is equivalent to the
// AgentServiceRegistration passed in.
func (t *ConsulTarget) shouldUpdateService(asr *capi.AgentServiceRegistration) (bool, error) {
	services, err := t.client.Agent().Services()
	if err != nil {
		return false, errors.Wrap(err, "Error getting agent services")
	}

	// Consul Service doesn't exist
	service, found := services[asr.ID]
	if !found {
		return true, nil
	}

	sort.Strings(asr.Tags)
	sort.Strings(service.Tags)

	// verify that the AgentService and AgentServiceRegistration are the same.
	// Because there is no API for it, this does not (and cannot) verify if the
	// Consul Agent Service *Check* has changed, but since the Check is
	// generated from metadata present in the AgentService, this should be fine.
	if asr.ID == service.ID &&
		asr.Name == service.Service &&
		reflect.DeepEqual(asr.Tags, service.Tags) &&
		asr.Port == service.Port &&
		asr.Address == service.Address {
		return false, nil
	}

	return true, nil
}

func (t *ConsulTarget) WriteNodes(nodes []*v1.Node) error {
	var exportedNodes exportedNodeList
	var err error
	tags := []string{"kv:nodes"}

	if !t.elector.IsLeader() {
		// do nothing
		return nil
	}

	for _, k8sNode := range nodes {
		for _, addr := range k8sNode.Status.Addresses {
			if addr.Type != v1.NodeInternalIP {
				continue
			}

			exportedNode := ExportedNode{
				Name:    k8sNode.Name,
				Address: addr.Address,
			}
			exportedNodes = append(exportedNodes, exportedNode)
		}
	}

	sort.Sort(exportedNodes)

	nodeJson, err := json.Marshal(exportedNodes)
	if err != nil {
		return errors.Wrap(err, "Error marshaling node JSON")
	}

	key := fmt.Sprintf("%s/nodes/%s", t.kvPrefix, t.clusterId)

	var current *capi.KVPair
	stats.WithTiming("consul.kv.time", append(tags, "method:get"), func() {
		current, _, err = t.client.KV().Get(key, &capi.QueryOptions{})
	})
	if err != nil {
		return errors.Wrapf(err, "Error getting %s key", key)
	}

	if current != nil && bytes.Equal(current.Value, nodeJson) {
		// nothing changed
		return nil
	}

	kv := capi.KVPair{
		Key:   key,
		Value: nodeJson,
	}

	stats.WithTiming("consul.kv.time", append(tags, "method:put"), func() {
		_, err = t.client.KV().Put(&kv, nil)
	})

	if err != nil {
		return errors.Wrapf(err, "Error writing %s key", key)
	}

	log.Println("[LEADER] Writing Node list to ", key)
	return nil
}

func (esl exportedNodeList) Len() int {
	return len(esl)
}

func (esl exportedNodeList) Swap(i, j int) {
	esl[i], esl[j] = esl[j], esl[i]
}

func (esl exportedNodeList) Less(i, j int) bool {
	return esl[i].Name < esl[j].Name
}
