// This package wraps selected methods from the Consul API and submits stats
// via the stats package
package consul

import (
	"github.com/github/kube-service-exporter/pkg/stats"
	capi "github.com/hashicorp/consul/api"
)

type InstrumentedKV struct {
	client *capi.Client
}

type InstrumentedAgent struct {
	client *capi.Client
}

func NewInstrumentedKV(client *capi.Client) *InstrumentedKV {
	return &InstrumentedKV{client: client}
}

func (kv *InstrumentedKV) Get(key string, q *capi.QueryOptions, tags []string) (*capi.KVPair, *capi.QueryMeta, error) {
	var kvPair *capi.KVPair
	var meta *capi.QueryMeta
	var err error

	all_tags := append(tags, "method:get")

	stats.WithTiming("consul.kv.time", all_tags, func() {
		kvPair, meta, err = kv.client.KV().Get(key, q)
	})
	stats.IncrSuccessOrFail(err, "consul.kv", all_tags)
	return kvPair, meta, err
}

func (kv *InstrumentedKV) Put(p *capi.KVPair, q *capi.WriteOptions, tags []string) (*capi.WriteMeta, error) {
	var meta *capi.WriteMeta
	var err error

	all_tags := append(tags, "method:put")

	stats.WithTiming("consul.kv.time", all_tags, func() {
		meta, err = kv.client.KV().Put(p, q)
	})
	stats.IncrSuccessOrFail(err, "consul.kv", all_tags)

	return meta, err
}

func (kv *InstrumentedKV) DeleteTree(prefix string, w *capi.WriteOptions, tags []string) (*capi.WriteMeta, error) {
	var meta *capi.WriteMeta
	var err error

	all_tags := append(tags, "method:delete")

	stats.WithTiming("consul.kv.time", all_tags, func() {
		meta, err = kv.client.KV().DeleteTree(prefix, w)
	})
	stats.IncrSuccessOrFail(err, "consul.kv", all_tags)

	return meta, err
}

func NewInstrumentedAgent(client *capi.Client) *InstrumentedAgent {
	return &InstrumentedAgent{client: client}
}

func (a *InstrumentedAgent) ServiceRegister(service *capi.AgentServiceRegistration) error {
	var err error
	tags := []string{"method:put", "service:" + service.ID}

	stats.WithTiming("consul.service.time", tags, func() {
		err = a.client.Agent().ServiceRegister(service)
	})
	stats.IncrSuccessOrFail(err, "consul.service", tags)

	return err
}

func (a *InstrumentedAgent) ServiceDeregister(serviceID string) error {
	var err error
	tags := []string{"method:delete", "service:" + serviceID}

	stats.WithTiming("consul.service.time", tags, func() {
		err = a.client.Agent().ServiceDeregister(serviceID)
	})
	stats.IncrSuccessOrFail(err, "consul.service", tags)

	return err
}

func (a *InstrumentedAgent) Services() (map[string]*capi.AgentService, error) {
	var err error
	var services map[string]*capi.AgentService
	tags := []string{"method:get"}

	stats.WithTiming("consul.service.time", tags, func() {
		services, err = a.client.Agent().Services()
	})
	stats.IncrSuccessOrFail(err, "consul.service", tags)

	return services, err
}
