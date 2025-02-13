package gpudmanager

import (
	"context"

	"github.com/leptonai/gpud/pkg/gpud-manager/controllers"
	"github.com/leptonai/gpud/pkg/gpud-manager/informer"
	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
)

type Manager struct {
	packageController *controllers.PackageController
}

var GlobalController *controllers.PackageController

func New() (*Manager, error) {
	return &Manager{}, nil
}

func (a *Manager) Start(ctx context.Context) {
	watcher := informer.NewFileInformer()
	packageController := controllers.NewPackageController(watcher)
	_ = packageController.Run(ctx)
	a.packageController = packageController
	GlobalController = packageController
}

func (a *Manager) Status(ctx context.Context) ([]packages.PackageStatus, error) {
	return a.packageController.Status(ctx)
}
