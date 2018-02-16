package controller

type ConsulTarget struct {
}

var _ ExportTarget = (*ConsulTarget)(nil)

func (t *ConsulTarget) Create(es *ExportedService) (bool, error) {
	return true, nil
}

func (t *ConsulTarget) Update(es *ExportedService) (bool, error) {
	return true, nil
}

func (t *ConsulTarget) Delete(es *ExportedService) (bool, error) {
	return true, nil
}
