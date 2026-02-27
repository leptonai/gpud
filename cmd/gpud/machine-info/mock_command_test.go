package machineinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	apiv1 "github.com/leptonai/gpud/api/v1"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/netutil"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/providers"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-machine-info-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("state-file", "", "")
	_ = flags.String("output-format", gpudcommon.OutputFormatPlain, "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)
	stderrR, stderrW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = stdoutW
	os.Stderr = stderrW
	t.Cleanup(func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	stdoutDone := make(chan string, 1)
	go func(reader *os.File) {
		b, _ := io.ReadAll(reader)
		stdoutDone <- string(b)
	}(stdoutR)

	stderrDone := make(chan string, 1)
	go func(reader *os.File) {
		b, _ := io.ReadAll(reader)
		stderrDone <- string(b)
	}(stderrR)

	fn()

	require.NoError(t, stdoutW.Close())
	require.NoError(t, stderrW.Close())
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	stdout := <-stdoutDone
	stderr := <-stderrDone
	require.NoError(t, stdoutR.Close())
	require.NoError(t, stderrR.Close())

	return stdout, stderr
}

// TestCommand_InvalidLogLevel tests when an invalid log level is provided.
func TestCommand_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-machine-info-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("state-file", "", "")
		_ = flags.String("output-format", gpudcommon.OutputFormatPlain, "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := Command(cliContext)
		require.Error(t, err)
	})
}

// TestCommand_StateFileError tests when getting state file fails.
func TestCommand_StateFileError(t *testing.T) {
	mockey.PatchConvey("command state file error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("failed to get state file")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get state file")
	})
}

// TestCommand_NVMLError tests when NVML initialization fails.
func TestCommand_NVMLError(t *testing.T) {
	mockey.PatchConvey("command nvml error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return nil, errors.New("NVML not available")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NVML not available")
	})
}

// TestCommand_GetMachineInfoError tests when GetMachineInfo fails.
func TestCommand_GetMachineInfoError(t *testing.T) {
	mockey.PatchConvey("command get machine info error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return nil, errors.New("failed to get machine info")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get machine info")
	})
}

// TestCommand_Success tests successful command execution without state file.
func TestCommand_Success(t *testing.T) {
	mockey.PatchConvey("command success without state file", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
			}, nil
		}).Build()

		mockey.Mock(netutil.PublicIP).To(func() (string, error) {
			return "", errors.New("no public IP")
		}).Build()

		mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_SuccessWithProvider tests successful command execution with provider info.
func TestCommand_SuccessWithProvider(t *testing.T) {
	mockey.PatchConvey("command success with provider", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
			}, nil
		}).Build()

		mockey.Mock(netutil.PublicIP).To(func() (string, error) {
			return "1.2.3.4", nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
			return &providers.Info{
				Provider:  "aws",
				PublicIP:  publicIP,
				PrivateIP: "10.0.0.1",
			}
		}).Build()
		mockey.Mock(pkgmachineinfo.GetMachineLocation).To(func() *apiv1.MachineLocation {
			return &apiv1.MachineLocation{Region: "us-east-1"}
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_SuccessWithProviderNoPrivateIP tests when provider info has no private IP.
func TestCommand_SuccessWithProviderNoPrivateIP(t *testing.T) {
	mockey.PatchConvey("command success with provider no private IP", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				NICInfo:   nil,
			}, nil
		}).Build()

		mockey.Mock(netutil.PublicIP).To(func() (string, error) {
			return "1.2.3.4", nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
			return &providers.Info{
				Provider:  "gcp",
				PublicIP:  publicIP,
				PrivateIP: "", // No private IP
			}
		}).Build()
		mockey.Mock(pkgmachineinfo.GetMachineLocation).To(func() *apiv1.MachineLocation {
			return &apiv1.MachineLocation{Region: "us-west-2"}
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

func TestCommand_FillsProviderPrivateIPFromMachineNIC(t *testing.T) {
	mockey.PatchConvey("command fills provider private ip from machine nic info", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
				NICInfo: &apiv1.MachineNICInfo{
					PrivateIPInterfaces: []apiv1.MachineNetworkInterface{
						{IP: "", Addr: netip.MustParseAddr("10.0.0.7")},
						{IP: "8.8.8.8", Addr: netip.MustParseAddr("8.8.8.8")},
						{IP: "10.0.0.42", Addr: netip.MustParseAddr("10.0.0.42")},
					},
				},
			}, nil
		}).Build()

		mockey.Mock(netutil.PublicIP).To(func() (string, error) {
			return "1.2.3.4", nil
		}).Build()

		providerInfo := &providers.Info{
			Provider:  "aws",
			PublicIP:  "1.2.3.4",
			PrivateIP: "",
			Region:    "us-east-1",
		}
		mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
			return providerInfo
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, "10.0.0.42", providerInfo.PrivateIP)
	})
}

// TestCommand_ProviderRegionFallbackToMachineLocation tests provider region display fallback.
func TestCommand_ProviderRegionFallbackToMachineLocation(t *testing.T) {
	mockey.PatchConvey("command fills empty provider region from machine location", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
			}, nil
		}).Build()

		mockey.Mock(netutil.PublicIP).To(func() (string, error) {
			return "46.148.127.98", nil
		}).Build()

		providerInfo := &providers.Info{
			Provider:  "nscale",
			PublicIP:  "46.148.127.98",
			PrivateIP: "7.247.195.146",
			Region:    "",
		}
		mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
			return providerInfo
		}).Build()
		mockey.Mock(pkgmachineinfo.GetMachineLocation).To(func() *apiv1.MachineLocation {
			return &apiv1.MachineLocation{Region: "eu-west-1"}
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, "eu-west-1", providerInfo.Region)
	})
}

// TestCommand_WithExistingStateFile tests command with existing state file.
func TestCommand_WithExistingStateFile(t *testing.T) {
	mockey.PatchConvey("command with existing state file", t, func() {
		// Create a temporary state file
		tmpDir := t.TempDir()
		stateFile := filepath.Join(tmpDir, "state.db")

		// Create an empty file to make os.Stat succeed
		f, err := os.Create(stateFile)
		require.NoError(t, err)
		_ = f.Close()

		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()

		// Mock sqlite.Open to return a mock database
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("mock db error")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err = Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestCommand_ReadMachineIDError tests when reading machine ID from DB fails.
func TestCommand_ReadMachineIDError(t *testing.T) {
	mockey.PatchConvey("command read machine ID error", t, func() {
		// Create a temporary state file
		tmpDir := t.TempDir()
		stateFile := filepath.Join(tmpDir, "state.db")

		// Create an empty file to make os.Stat succeed
		f, err := os.Create(stateFile)
		require.NoError(t, err)
		_ = f.Close()

		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return stateFile, nil
		}).Build()

		// Mock sqlite.Open to return a real in-memory database
		// Note: We must use a real *sql.DB (not &sql.DB{}) because the Command function
		// calls db.Close() in a defer statement, and Close() on an uninitialized
		// sql.DB causes a nil pointer dereference panic.
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return sql.Open("sqlite3", ":memory:")
		}).Build()

		// Mock ReadMachineID to return an error
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("failed to read machine ID")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err = Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read machine ID")
	})
}

// TestCommand_ValidLogLevels tests that valid log levels are accepted.
func TestCommand_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			mockey.PatchConvey("valid log level "+level, t, func() {
				mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
					return "/nonexistent/state.db", nil
				}).Build()

				mockNVML := nvidianvml.NewNoOp()
				mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
					return mockNVML, nil
				}).Build()

				mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
					return &apiv1.MachineInfo{
						MachineID: "test-machine-id",
					}, nil
				}).Build()

				mockey.Mock(netutil.PublicIP).To(func() (string, error) {
					return "", errors.New("no public IP")
				}).Build()

				mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
					return nil
				}).Build()

				app := cli.NewApp()
				flags := flag.NewFlagSet("gpud-machine-info-test", flag.ContinueOnError)
				flags.SetOutput(io.Discard)

				_ = flags.String("log-level", level, "")
				_ = flags.String("state-file", "", "")
				_ = flags.String("output-format", gpudcommon.OutputFormatPlain, "")

				require.NoError(t, flags.Parse([]string{"--log-level", level}))
				cliContext := cli.NewContext(app, flags, nil)

				err := Command(cliContext)
				require.NoError(t, err)
			})
		})
	}
}

func TestCommand_JSONOutput(t *testing.T) {
	mockey.PatchConvey("command json output", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
			}, nil
		}).Build()

		mockey.Mock(netutil.PublicIP).To(func() (string, error) {
			return "1.2.3.4", nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
			return &providers.Info{
				Provider:  "aws",
				PublicIP:  publicIP,
				PrivateIP: "10.0.0.1",
			}
		}).Build()
		mockey.Mock(pkgmachineinfo.GetMachineLocation).To(func() *apiv1.MachineLocation {
			return &apiv1.MachineLocation{Region: "us-east-1"}
		}).Build()

		cliContext := newCLIContext(t, []string{
			"--output-format", "json",
			"--log-level", "debug",
		})

		stdout, stderr := captureOutput(t, func() {
			require.NoError(t, Command(cliContext))
		})

		assert.Empty(t, stderr)
		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &out))
		assert.Equal(t, "", out["machine_id"])

		providerOut, ok := out["provider"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "aws", providerOut["provider"])
		assert.Equal(t, "us-east-1", providerOut["region"])
		assert.NotContains(t, stdout, "successfully found provider")
	})
}

func TestCommand_JSONOutputWithoutMachineLocation(t *testing.T) {
	mockey.PatchConvey("command json output without machine location fallback", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
			}, nil
		}).Build()

		mockey.Mock(netutil.PublicIP).To(func() (string, error) {
			return "1.2.3.4", nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
			return &providers.Info{
				Provider:  "aws",
				PublicIP:  publicIP,
				PrivateIP: "10.0.0.1",
			}
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineLocation).To(func() *apiv1.MachineLocation {
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{
			"--output-format", "json",
		})

		stdout, stderr := captureOutput(t, func() {
			require.NoError(t, Command(cliContext))
		})

		assert.Empty(t, stderr)
		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &out))

		providerOut, ok := out["provider"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "aws", providerOut["provider"])
		assert.Equal(t, "", providerOut["region"])
	})
}

func TestCommand_JSONOutputNilProvider(t *testing.T) {
	mockey.PatchConvey("command json output nil provider", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/nonexistent/state.db", nil
		}).Build()

		mockNVML := nvidianvml.NewNoOp()
		mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
			return mockNVML, nil
		}).Build()

		mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvmlInstance nvidianvml.Instance) (*apiv1.MachineInfo, error) {
			return &apiv1.MachineInfo{
				MachineID: "test-machine-id",
			}, nil
		}).Build()

		mockey.Mock(netutil.PublicIP).To(func() (string, error) {
			return "", errors.New("no public IP")
		}).Build()

		mockey.Mock(pkgmachineinfo.GetProvider).To(func(publicIP string) *providers.Info {
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{
			"--output-format", "json",
		})

		stdout, stderr := captureOutput(t, func() {
			require.NoError(t, Command(cliContext))
		})

		assert.Empty(t, stderr)
		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &out))
		assert.Contains(t, out, "provider")
		assert.Nil(t, out["provider"])
	})
}

func TestCommand_InvalidLogLevelJSON(t *testing.T) {
	mockey.PatchConvey("command invalid log level json", t, func() {
		cliContext := newCLIContext(t, []string{
			"--output-format", "json",
			"--log-level", "invalid-level",
		})

		err := Command(cliContext)
		require.Error(t, err)

		jerr, ok := gpudcommon.AsJSONCommandError(err)
		require.True(t, ok)
		assert.Equal(t, "invalid_log_level", jerr.Code())
		assert.NotEmpty(t, jerr.Error())
	})
}
