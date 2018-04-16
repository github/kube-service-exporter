package controller

type ExportTarget interface {
	Create(*ExportedService) (bool, error)
	Update(*ExportedService) (bool, error)
	Delete(*ExportedService) (bool, error)
	WriteNodes([]string) error
}
