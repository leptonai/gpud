package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
)

type unclosablePlugin struct{ nonClosableComponent }

func (*unclosablePlugin) IsCustomPlugin() bool        { return true }
func (*unclosablePlugin) Spec() pkgcustomplugins.Spec { return pkgcustomplugins.Spec{} }
func (*unclosablePlugin) Close() error                { return errors.New("close failed") }

func TestPluginRegistrationFailurePreservesBuiltins(t *testing.T) {
	instance := &components.GPUdInstance{RootCtx: context.Background()}
	s := &Server{gpudInstance: instance, componentsRegistry: components.NewRegistry(instance), initRegistry: components.NewRegistry(instance)}
	builtin := &nonClosableComponent{name: "builtin-collision"}
	_, err := s.componentsRegistry.Register(func(*components.GPUdInstance) (components.Component, error) { return builtin, nil })
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "plugins.yaml")
	_, err = pkgcustomplugins.SaveSpecs(path, pkgcustomplugins.Specs{{PluginName: builtin.Name(), PluginType: pkgcustomplugins.SpecTypeComponent, HealthStatePlugin: &pkgcustomplugins.Plugin{}}})
	require.NoError(t, err)
	err = s.loadPluginSpecs(path)
	require.Error(t, err)
	s.quarantinePluginSpecs(path, err)
	require.Same(t, builtin, s.componentsRegistry.Get(builtin.Name()))
	require.Len(t, s.componentsRegistry.All(), 1)
	require.FileExists(t, path+".invalid")
}

func TestPluginQuarantineFailureStillRemovesPlugins(t *testing.T) {
	instance := &components.GPUdInstance{RootCtx: context.Background()}
	s := &Server{componentsRegistry: components.NewRegistry(instance), initRegistry: components.NewRegistry(instance)}
	plugin := &unclosablePlugin{nonClosableComponent{name: "quarantine-close-failure"}}
	_, err := s.componentsRegistry.Register(func(*components.GPUdInstance) (components.Component, error) { return plugin, nil })
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "missing.yaml")
	require.NotPanics(t, func() { s.quarantinePluginSpecs(path, errors.New("profile failed")) })
	require.Empty(t, s.componentsRegistry.All())
}

func TestUnmanagedPluginLoadFailureRemainsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.yaml")
	content := []byte("[malformed YAML")
	require.NoError(t, os.WriteFile(path, content, 0600))
	s := &Server{}
	require.ErrorContains(t, s.loadPluginSpecs(path), "failed to load plugin specs")
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, got)
	require.NoFileExists(t, path+".invalid")
}
