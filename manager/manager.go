package manager

import (
	"context"

	"github.com/leptonai/gpud/manager/controllers"
	"github.com/leptonai/gpud/manager/informer"
)

type Manager struct {
}

func New() (*Manager, error) {
	return &Manager{}, nil
}

func (a *Manager) Start(ctx context.Context) {
	watcher := informer.NewFileInformer()
	packageController := controllers.NewPackageController(watcher)
	_ = packageController.Run(ctx)
}
