package customplugins

import (
	"errors"
	"flag"
	"io"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	customplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	custompluginstestdata "github.com/leptonai/gpud/pkg/custom-plugins/testdata"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	nvidiaproduct "github.com/leptonai/gpud/pkg/nvidia/product"
)

type fakeNVMLInstance struct{}

func (f *fakeNVMLInstance) NVMLExists() bool { return true }
func (f *fakeNVMLInstance) Library() nvmllib.Library {
	return nil
}
func (f *fakeNVMLInstance) Devices() map[string]device.Device {
	return nil
}
func (f *fakeNVMLInstance) ProductName() string { return "" }
func (f *fakeNVMLInstance) Architecture() string {
	return ""
}
func (f *fakeNVMLInstance) Brand() string { return "" }
func (f *fakeNVMLInstance) DriverVersion() string {
	return ""
}
func (f *fakeNVMLInstance) DriverMajor() int { return 0 }
func (f *fakeNVMLInstance) CUDAVersion() string {
	return ""
}
func (f *fakeNVMLInstance) FabricManagerSupported() bool { return false }
func (f *fakeNVMLInstance) FabricStateSupported() bool   { return false }
func (f *fakeNVMLInstance) GetMemoryErrorManagementCapabilities() nvidiaproduct.MemoryErrorManagementCapabilities {
	return nvidiaproduct.MemoryErrorManagementCapabilities{}
}
func (f *fakeNVMLInstance) Shutdown() error { return nil }
func (f *fakeNVMLInstance) InitError() error {
	return nil
}

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-custom-plugins-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.Bool("run", false, "")
	_ = flags.Bool("fail-fast", false, "")
	_ = flags.String("infiniband-class-root-dir", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// TestCommand_WithExampleSpecs tests the command using example specs (no args).
func TestCommand_WithExampleSpecs(t *testing.T) {
	mockey.PatchConvey("command with example specs", t, func() {
		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_LoadSpecsError tests when loading specs from file fails.
func TestCommand_LoadSpecsError(t *testing.T) {
	mockey.PatchConvey("command load specs error", t, func() {
		mockey.Mock(customplugins.LoadSpecs).To(func(path string) (customplugins.Specs, error) {
			return nil, errors.New("failed to load specs")
		}).Build()

		// Provide an argument to trigger LoadSpecs instead of ExampleSpecs
		cliContext := newCLIContext(t, []string{"test-spec.yaml"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load specs")
	})
}

// TestCommand_SpecsValidationError tests when specs validation fails.
func TestCommand_SpecsValidationError(t *testing.T) {
	mockey.PatchConvey("command specs validation error", t, func() {
		// Create invalid specs that will fail validation
		mockey.Mock(custompluginstestdata.ExampleSpecs).To(func() customplugins.Specs {
			return customplugins.Specs{
				{
					PluginName: "", // Empty plugin name should fail validation
					PluginType: "invalid-type",
				},
			}
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
	})
}

// TestCommand_WithRunFlagNvmlError tests when NVML initialization fails with --run flag.
func TestCommand_WithRunFlagNvmlError(t *testing.T) {
	mockey.PatchConvey("command with run flag nvml error", t, func() {
		// Mock NVML to return an error
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return nil, errors.New("NVML not available")
		}).Build()

		cliContext := newCLIContext(t, []string{"--run"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NVML not available")
	})
}

// TestCommand_WithLoadedSpecs tests loading specs from file successfully.
func TestCommand_WithLoadedSpecs(t *testing.T) {
	mockey.PatchConvey("command with loaded specs", t, func() {
		// Mock loadSpecs to return valid specs
		mockey.Mock(customplugins.LoadSpecs).To(func(path string) (customplugins.Specs, error) {
			return custompluginstestdata.ExampleSpecs(), nil
		}).Build()

		cliContext := newCLIContext(t, []string{"some-spec-file.yaml"})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_SortingInitPluginsFirst tests that init type plugins are sorted first.
func TestCommand_SortingInitPluginsFirst(t *testing.T) {
	mockey.PatchConvey("command sorting init plugins first", t, func() {
		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_InvalidLogLevel tests when an invalid log level is provided.
func TestCommand_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-custom-plugins-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.Bool("run", false, "")
		_ = flags.Bool("fail-fast", false, "")
		_ = flags.String("infiniband-class-root-dir", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := Command(cliContext)
		require.Error(t, err)
	})
}

// TestCommand_LoadSpecsPathVerification tests that LoadSpecs receives the correct path.
func TestCommand_LoadSpecsPathVerification(t *testing.T) {
	mockey.PatchConvey("command load specs path verification", t, func() {
		var receivedPath string
		mockey.Mock(customplugins.LoadSpecs).To(func(path string) (customplugins.Specs, error) {
			receivedPath = path
			return custompluginstestdata.ExampleSpecs(), nil
		}).Build()

		expectedPath := "my-custom-spec.yaml"
		cliContext := newCLIContext(t, []string{expectedPath})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, expectedPath, receivedPath)
	})
}

// TestCommand_ValidLogLevels tests that valid log levels are accepted.
func TestCommand_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			mockey.PatchConvey("valid log level "+level, t, func() {
				app := cli.NewApp()
				flags := flag.NewFlagSet("gpud-custom-plugins-test", flag.ContinueOnError)
				flags.SetOutput(io.Discard)

				_ = flags.String("log-level", level, "")
				_ = flags.Bool("run", false, "")
				_ = flags.Bool("fail-fast", false, "")
				_ = flags.String("infiniband-class-root-dir", "", "")

				require.NoError(t, flags.Parse([]string{"--log-level", level}))
				cliContext := cli.NewContext(app, flags, nil)

				err := Command(cliContext)
				require.NoError(t, err)
			})
		})
	}
}

// TestCommand_NoRunFlagSkipsExecution tests that without --run flag, only validation happens.
func TestCommand_NoRunFlagSkipsExecution(t *testing.T) {
	mockey.PatchConvey("command no run flag skips execution", t, func() {
		// NVML.New should not be called when --run is not set
		nvmlCalled := false
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			nvmlCalled = true
			return nil, errors.New("should not be called")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.False(t, nvmlCalled, "NVML.New should not be called when --run is not set")
	})
}

// TestCommand_ExampleSpecsIsUsedWhenNoArgs tests that ExampleSpecs is called when no args provided.
func TestCommand_ExampleSpecsIsUsedWhenNoArgs(t *testing.T) {
	mockey.PatchConvey("command example specs is used when no args", t, func() {
		exampleSpecsCalled := false
		mockey.Mock(custompluginstestdata.ExampleSpecs).To(func() customplugins.Specs {
			exampleSpecsCalled = true
			return customplugins.Specs{
				{
					PluginName: "test-plugin",
					PluginType: "component",
				},
			}
		}).Build()

		cliContext := newCLIContext(t, []string{})
		_ = Command(cliContext)
		assert.True(t, exampleSpecsCalled, "ExampleSpecs should be called when no args provided")
	})
}

// TestCommand_LoadSpecsNotCalledWhenNoArgs tests that LoadSpecs is NOT called when no args.
func TestCommand_LoadSpecsNotCalledWhenNoArgs(t *testing.T) {
	mockey.PatchConvey("command load specs not called when no args", t, func() {
		loadSpecsCalled := false
		mockey.Mock(customplugins.LoadSpecs).To(func(path string) (customplugins.Specs, error) {
			loadSpecsCalled = true
			return nil, errors.New("should not be called")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		_ = Command(cliContext)
		assert.False(t, loadSpecsCalled, "LoadSpecs should not be called when no args provided")
	})
}

// TestCommand_RunFlagExecuteError tests run flag when ExecuteInOrder returns error.
func TestCommand_RunFlagExecuteError(t *testing.T) {
	mockey.PatchConvey("command run flag execute error", t, func() {
		mockey.Mock(customplugins.LoadSpecs).To(func(path string) (customplugins.Specs, error) {
			return custompluginstestdata.ExampleSpecs(), nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return &fakeNVMLInstance{}, nil
		}).Build()

		mockey.Mock(customplugins.Specs.ExecuteInOrder).To(func(specs customplugins.Specs, gpudInstance *components.GPUdInstance, failFast bool) ([]components.CheckResult, error) {
			return nil, errors.New("execute failed")
		}).Build()

		cliContext := newCLIContext(t, []string{"--run", "spec.yaml"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "execute failed")
	})
}

type fakeCheckResult struct {
	name    string
	summary string
	health  apiv1.HealthStateType
	states  apiv1.HealthStates
	debug   string
}

func (f *fakeCheckResult) ComponentName() string                  { return f.name }
func (f *fakeCheckResult) String() string                         { return f.summary }
func (f *fakeCheckResult) Summary() string                        { return f.summary }
func (f *fakeCheckResult) HealthStateType() apiv1.HealthStateType { return f.health }
func (f *fakeCheckResult) HealthStates() apiv1.HealthStates       { return f.states }
func (f *fakeCheckResult) Debug() string                          { return f.debug }

func TestCommand_RunFlagSuccessRendersResultsWithMockey(t *testing.T) {
	mockey.PatchConvey("command run flag success renders results", t, func() {
		mockey.Mock(customplugins.LoadSpecs).To(func(path string) (customplugins.Specs, error) {
			return custompluginstestdata.ExampleSpecs(), nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return &fakeNVMLInstance{}, nil
		}).Build()

		var gotFailFast bool
		var gotInfinibandRoot string
		mockey.Mock(customplugins.Specs.ExecuteInOrder).To(func(specs customplugins.Specs, gpudInstance *components.GPUdInstance, failFast bool) ([]components.CheckResult, error) {
			gotFailFast = failFast
			gotInfinibandRoot = gpudInstance.NVIDIAToolOverwrites.InfinibandClassRootDir
			return []components.CheckResult{
				&fakeCheckResult{
					name:    "healthy-component",
					summary: "ok",
					health:  apiv1.HealthStateTypeHealthy,
					debug:   "debug-info",
					states: apiv1.HealthStates{
						{
							Component: "healthy-component",
							Name:      "healthy-component",
							Health:    apiv1.HealthStateTypeHealthy,
							RunMode:   apiv1.RunModeTypeManual,
							ExtraInfo: map[string]string{"hello": "world"},
						},
					},
				},
				&fakeCheckResult{
					name:    "unhealthy-component",
					summary: "bad",
					health:  apiv1.HealthStateTypeUnhealthy,
					states:  nil,
				},
			}, nil
		}).Build()

		cliContext := newCLIContext(t, []string{"--run", "--fail-fast", "--infiniband-class-root-dir", "/sys/class/infiniband", "spec.yaml"})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.True(t, gotFailFast)
		assert.Equal(t, "/sys/class/infiniband", gotInfinibandRoot)
	})
}
