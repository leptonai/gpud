package library

import (
	"context"
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/file"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

type fakeNVMLInstanceLibrary struct{}

func (f *fakeNVMLInstanceLibrary) NVMLExists() bool { return true }
func (f *fakeNVMLInstanceLibrary) Library() nvmllib.Library {
	return nil
}
func (f *fakeNVMLInstanceLibrary) Devices() map[string]device.Device { return nil }
func (f *fakeNVMLInstanceLibrary) ProductName() string               { return "" }
func (f *fakeNVMLInstanceLibrary) Architecture() string              { return "" }
func (f *fakeNVMLInstanceLibrary) Brand() string                     { return "" }
func (f *fakeNVMLInstanceLibrary) DriverVersion() string             { return "" }
func (f *fakeNVMLInstanceLibrary) DriverMajor() int                  { return 0 }
func (f *fakeNVMLInstanceLibrary) CUDAVersion() string               { return "" }
func (f *fakeNVMLInstanceLibrary) FabricManagerSupported() bool      { return false }
func (f *fakeNVMLInstanceLibrary) FabricStateSupported() bool        { return false }
func (f *fakeNVMLInstanceLibrary) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (f *fakeNVMLInstanceLibrary) Shutdown() error  { return nil }
func (f *fakeNVMLInstanceLibrary) InitError() error { return nil }

func TestCheck_WithNVMLMissingLibrariesWithMockey(t *testing.T) {
	mockey.PatchConvey("Check marks unhealthy when default libraries missing", t, func() {
		gpudInstance := &components.GPUdInstance{
			RootCtx:      context.Background(),
			NVMLInstance: &fakeNVMLInstanceLibrary{},
		}
		comp, err := New(gpudInstance)
		require.NoError(t, err)

		mockey.Mock(file.FindLibrary).To(func(name string, opts ...file.OpOption) (string, error) {
			return "", file.ErrLibraryNotFound
		}).Build()

		result := comp.Check()
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthStateType())
		assert.Contains(t, result.Summary(), "library")
	})
}

func TestCheckResult_StringMarshalErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("checkResult String returns marshal error", t, func() {
		mockey.Mock(yaml.Marshal).To(func(_ any) ([]byte, error) {
			return nil, errors.New("marshal failed")
		}).Build()

		cr := &checkResult{reason: "ok"}
		assert.Contains(t, cr.String(), "error marshaling data")
	})
}

var _ nvidianvml.Instance = &fakeNVMLInstanceLibrary{}
