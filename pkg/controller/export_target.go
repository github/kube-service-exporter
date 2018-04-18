package controller

import "k8s.io/api/core/v1"

type ExportTarget interface {
	Create(*ExportedService) (bool, error)
	Update(*ExportedService) (bool, error)
	Delete(*ExportedService) (bool, error)
	WriteNodes([]v1.Node) error
}
