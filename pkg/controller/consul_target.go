package controller

import (
	"fmt"

	capi "github.com/hashicorp/consul/api"
)

type ConsulTarget struct {
	client *capi.Client
	hostIP string
}

var _ ExportTarget = (*ConsulTarget)(nil)

func NewConsulTarget(hostIP string) (*ConsulTarget, error) {
	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		return nil, err
	}

	return &ConsulTarget{client: client, hostIP: hostIP}, nil
}

func (t *ConsulTarget) Create(es *ExportedService) (bool, error) {
	asr := t.asrFromExportedService(es)
	err := t.client.Agent().ServiceRegister(asr)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (t *ConsulTarget) Update(es *ExportedService) (bool, error) {
	ok, err := t.Create(es)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (t *ConsulTarget) Delete(es *ExportedService) (bool, error) {
	err := t.client.Agent().ServiceDeregister(es.Id())
	if err != nil {
		return false, err
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
