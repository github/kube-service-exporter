package controller

import (
	"fmt"

	capi "github.com/hashicorp/consul/api"
)

type ConsulTarget struct {
	client *capi.Client
}

var _ ExportTarget = (*ConsulTarget)(nil)

func NewConsulTarget() (*ConsulTarget, error) {
	client, err := capi.NewClient(capi.DefaultConfig())
	if err != nil {
		return nil, err
	}

	return &ConsulTarget{client: client}, nil
}

func (t *ConsulTarget) Create(es *ExportedService) (bool, error) {
	asr := &capi.AgentServiceRegistration{
		Name: es.Id(),
		Tags: []string{es.ClusterId},
		Port: int(es.Port),
		Check: &capi.AgentServiceCheck{
			Name:     "NodePort",
			TCP:      fmt.Sprintf("127.0.0.1:%d", es.Port),
			Interval: "10s",
		},
	}

	agent := t.client.Agent()
	err := agent.ServiceRegister(asr)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (t *ConsulTarget) Update(es *ExportedService) (bool, error) {
	return true, nil
}

func (t *ConsulTarget) Delete(es *ExportedService) (bool, error) {
	return true, nil
}
