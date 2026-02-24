//go:build linux

package run

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	componentssxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	"github.com/leptonai/gpud/pkg/config"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	gpudserver "github.com/leptonai/gpud/pkg/server"
	pkgsystemd "github.com/leptonai/gpud/pkg/systemd"
)

type cliFlagValues struct {
	stringFlags   map[string]string
	boolFlags     map[string]bool
	intFlags      map[string]int
	durationFlags map[string]time.Duration
}

func newTestCLIContext(t *testing.T, values cliFlagValues) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("gpud-test", flag.ContinueOnError)

	set.String("log-level", "", "")
	set.String("log-file", "", "")
	set.String("data-dir", "", "")
	set.Bool("db-in-memory", false, "")
	set.Int("gpu-count", 0, "")
	set.String("endpoint", "", "")
	set.String("token", "", "")
	set.String("machine-id", "", "")
	set.String("listen-address", "", "")
	set.Bool("pprof", false, "")
	set.Duration("metrics-retention-period", 0, "")
	set.Duration("retention-period", 0, "")
	set.Duration("events-retention-period", 0, "")
	set.Bool("enable-auto-update", false, "")
	set.Int("auto-update-exit-code", 0, "")
	set.String("version-file", "", "")
	set.String("plugin-specs-file", "", "")
	set.Bool("skip-session-update-config", false, "")
	set.String("infiniband-class-root-dir", "", "")
	set.String("infiniband-exclude-devices", "", "")
	set.String("components", "", "")
	set.String("infiniband-expected-port-states", "", "")
	set.String("nvlink-expected-link-states", "", "")
	set.String("nfs-checker-configs", "", "")
	set.Int("xid-reboot-threshold", 0, "")
	set.Duration("xid-lookback-period", 0, "")
	set.Duration("sxid-lookback-period", 0, "")
	set.Int("threshold-celsius-slowdown-margin", 0, "")
	set.String("gpu-uuids-with-row-remapping-pending", "", "")
	set.String("gpu-uuids-with-row-remapping-failed", "", "")
	set.String("gpu-uuids-with-hw-slowdown", "", "")
	set.String("gpu-uuids-with-hw-slowdown-thermal", "", "")
	set.String("gpu-uuids-with-hw-slowdown-power-brake", "", "")
	set.String("gpu-uuids-with-gpu-lost", "", "")
	set.String("gpu-uuids-with-gpu-requires-reset", "", "")
	set.String("gpu-uuids-with-fabric-state-health-summary-unhealthy", "", "")
	set.String("gpu-product-name", "", "")
	set.Bool("nvml-device-get-devices-error", false, "")

	for key, val := range values.stringFlags {
		require.NoError(t, set.Set(key, val))
	}
	for key, val := range values.boolFlags {
		require.NoError(t, set.Set(key, strconv.FormatBool(val)))
	}
	for key, val := range values.intFlags {
		require.NoError(t, set.Set(key, strconv.Itoa(val)))
	}
	for key, val := range values.durationFlags {
		require.NoError(t, set.Set(key, val.String()))
	}

	app := cli.NewApp()
	return cli.NewContext(app, set, nil)
}

func TestCommand_SuccessPath(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"log-level":                              "info",
			"log-file":                               filepath.Join(tmpDir, "gpud.log"),
			"data-dir":                               tmpDir,
			"endpoint":                               "https://control-plane.example.com",
			"token":                                  "registration-token",
			"machine-id":                             "machine-123",
			"listen-address":                         "127.0.0.1:9999",
			"version-file":                           filepath.Join(tmpDir, "target_version"),
			"plugin-specs-file":                      filepath.Join(tmpDir, "plugins.json"),
			"infiniband-class-root-dir":              "/sys/class/infiniband",
			"infiniband-exclude-devices":             "mlx5_0, mlx5_1",
			"components":                             "nvidia,xid",
			"infiniband-expected-port-states":        `{"at_least_ports":2,"at_least_rate":100}`,
			"nvlink-expected-link-states":            `{"at_least_gpus_with_all_links_feature_enabled":1}`,
			"nfs-checker-configs":                    `[{"volume_path":"/tmp","dir_name":".gpud-nfs","file_contents":"ok"}]`,
			"gpu-uuids-with-row-remapping-pending":   "GPU-AAA,GPU-BBB",
			"gpu-uuids-with-row-remapping-failed":    "GPU-CCC",
			"gpu-uuids-with-hw-slowdown":             "GPU-DDD",
			"gpu-uuids-with-hw-slowdown-thermal":     "GPU-EEE",
			"gpu-uuids-with-hw-slowdown-power-brake": "GPU-FFF",
			"gpu-uuids-with-gpu-lost":                "GPU-GGG",
			"gpu-uuids-with-gpu-requires-reset":      "GPU-HHH",
			"gpu-uuids-with-fabric-state-health-summary-unhealthy": "GPU-III",
			"gpu-product-name": "H100-SXM",
		},
		boolFlags: map[string]bool{
			"pprof":                         true,
			"enable-auto-update":            true,
			"skip-session-update-config":    true,
			"nvml-device-get-devices-error": true,
		},
		intFlags: map[string]int{
			"gpu-count":                         4,
			"auto-update-exit-code":             42,
			"xid-reboot-threshold":              5,
			"threshold-celsius-slowdown-margin": 10,
		},
		durationFlags: map[string]time.Duration{
			"metrics-retention-period": 5 * time.Minute,
			"events-retention-period":  14 * 24 * time.Hour,
		},
	})

	mockey.PatchConvey("Command success path", t, func() {
		var receivedCfg *config.Config

		mockey.Mock(login.Login).To(func(ctx context.Context, cfg login.LoginConfig) error {
			return nil
		}).Build()
		mockey.Mock(recordLoginSuccessState).To(func(ctx context.Context, dataDir string) error {
			return nil
		}).Build()
		mockey.Mock(gpudmanager.New).To(func(dataDir string) (*gpudmanager.Manager, error) {
			return &gpudmanager.Manager{}, nil
		}).Build()
		mockey.Mock((*gpudmanager.Manager).Start).To(func(_ *gpudmanager.Manager, _ context.Context) error {
			return nil
		}).Build()
		mockey.Mock(gpudserver.New).To(func(_ context.Context, _ log.AuditLogger, cfg *config.Config, _ *gpudmanager.Manager) (*gpudserver.Server, error) {
			receivedCfg = cfg
			return &gpudserver.Server{}, nil
		}).Build()
		mockey.Mock(gpudserver.HandleSignals).To(func(_ context.Context, _ context.CancelFunc, _ chan os.Signal, _ chan gpudserver.ServerStopper, _ func(context.Context) error) chan struct{} {
			done := make(chan struct{})
			close(done)
			return done
		}).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return true
		}).Build()
		mockey.Mock(pkgsystemd.NotifyReady).To(func(_ context.Context) error {
			return nil
		}).Build()

		err := Command(ctx)
		require.NoError(t, err)
		require.NotNil(t, receivedCfg)
		assert.Equal(t, 5*time.Minute, receivedCfg.RetentionPeriod.Duration)
		assert.Equal(t, 14*24*time.Hour, receivedCfg.EventsRetentionPeriod.Duration)
	})
}

func TestCommand_NoToken_SystemctlMissing(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"log-level":                       "debug",
			"data-dir":                        tmpDir,
			"infiniband-expected-port-states": `{"at_least_ports":1,"at_least_rate":50}`,
			"nvlink-expected-link-states":     `{"at_least_gpus_with_all_links_feature_enabled":0}`,
			"nfs-checker-configs":             `[]`,
		},
		boolFlags: map[string]bool{
			"db-in-memory": true,
		},
		intFlags: map[string]int{
			"xid-reboot-threshold":              0,
			"threshold-celsius-slowdown-margin": 0,
		},
	})

	mockey.PatchConvey("Command skip login and systemctl", t, func() {
		mockey.Mock(gpudmanager.New).To(func(dataDir string) (*gpudmanager.Manager, error) {
			return &gpudmanager.Manager{}, nil
		}).Build()
		mockey.Mock((*gpudmanager.Manager).Start).To(func(_ *gpudmanager.Manager, _ context.Context) error {
			return nil
		}).Build()
		mockey.Mock(gpudserver.New).To(func(_ context.Context, _ log.AuditLogger, _ *config.Config, _ *gpudmanager.Manager) (*gpudserver.Server, error) {
			return &gpudserver.Server{}, nil
		}).Build()
		mockey.Mock(gpudserver.HandleSignals).To(func(_ context.Context, _ context.CancelFunc, _ chan os.Signal, _ chan gpudserver.ServerStopper, _ func(context.Context) error) chan struct{} {
			done := make(chan struct{})
			close(done)
			return done
		}).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		err := Command(ctx)
		require.NoError(t, err)
	})
}

func TestCommand_SetLookbackPeriods(t *testing.T) {
	tmpDir := t.TempDir()

	originalXidLookback := componentsxid.GetLookbackPeriod()
	originalSxidLookback := componentssxid.GetLookbackPeriod()
	t.Cleanup(func() {
		componentsxid.SetLookbackPeriod(originalXidLookback)
		componentssxid.SetLookbackPeriod(originalSxidLookback)
	})

	newXidLookback := 6 * time.Hour
	newSxidLookback := 8 * time.Hour

	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"log-level": "info",
			"data-dir":  tmpDir,
		},
		durationFlags: map[string]time.Duration{
			"xid-lookback-period":  newXidLookback,
			"sxid-lookback-period": newSxidLookback,
		},
	})

	mockey.PatchConvey("Command sets xid and sxid lookback periods", t, func() {
		mockey.Mock(gpudmanager.New).To(func(dataDir string) (*gpudmanager.Manager, error) {
			return &gpudmanager.Manager{}, nil
		}).Build()
		mockey.Mock((*gpudmanager.Manager).Start).To(func(_ *gpudmanager.Manager, _ context.Context) error {
			return nil
		}).Build()
		mockey.Mock(gpudserver.New).To(func(_ context.Context, _ log.AuditLogger, _ *config.Config, _ *gpudmanager.Manager) (*gpudserver.Server, error) {
			return &gpudserver.Server{}, nil
		}).Build()
		mockey.Mock(gpudserver.HandleSignals).To(func(_ context.Context, _ context.CancelFunc, _ chan os.Signal, _ chan gpudserver.ServerStopper, _ func(context.Context) error) chan struct{} {
			done := make(chan struct{})
			close(done)
			return done
		}).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		err := Command(ctx)
		require.NoError(t, err)
		assert.Equal(t, newXidLookback, componentsxid.GetLookbackPeriod())
		assert.Equal(t, newSxidLookback, componentssxid.GetLookbackPeriod())
	})
}

func TestCommand_UsesEventsRetentionForXidAndSxidLookback(t *testing.T) {
	tmpDir := t.TempDir()

	originalXidLookback := componentsxid.GetLookbackPeriod()
	originalSxidLookback := componentssxid.GetLookbackPeriod()
	t.Cleanup(func() {
		componentsxid.SetLookbackPeriod(originalXidLookback)
		componentssxid.SetLookbackPeriod(originalSxidLookback)
	})

	eventsRetention := 12 * time.Hour

	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"log-level": "info",
			"data-dir":  tmpDir,
		},
		durationFlags: map[string]time.Duration{
			"events-retention-period": eventsRetention,
		},
	})

	mockey.PatchConvey("Command uses events retention period as default xid/sxid lookback", t, func() {
		mockey.Mock(gpudmanager.New).To(func(dataDir string) (*gpudmanager.Manager, error) {
			return &gpudmanager.Manager{}, nil
		}).Build()
		mockey.Mock((*gpudmanager.Manager).Start).To(func(_ *gpudmanager.Manager, _ context.Context) error {
			return nil
		}).Build()
		mockey.Mock(gpudserver.New).To(func(_ context.Context, _ log.AuditLogger, _ *config.Config, _ *gpudmanager.Manager) (*gpudserver.Server, error) {
			return &gpudserver.Server{}, nil
		}).Build()
		mockey.Mock(gpudserver.HandleSignals).To(func(_ context.Context, _ context.CancelFunc, _ chan os.Signal, _ chan gpudserver.ServerStopper, _ func(context.Context) error) chan struct{} {
			done := make(chan struct{})
			close(done)
			return done
		}).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		err := Command(ctx)
		require.NoError(t, err)
		assert.Equal(t, eventsRetention, componentsxid.GetLookbackPeriod())
		assert.Equal(t, eventsRetention, componentssxid.GetLookbackPeriod())
	})
}

func TestCommand_DeprecatedRetentionPeriodFlagStillWorks(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"log-level": "info",
			"data-dir":  tmpDir,
		},
		durationFlags: map[string]time.Duration{
			"retention-period": 7 * time.Minute,
		},
	})

	mockey.PatchConvey("Command accepts deprecated retention-period flag", t, func() {
		var receivedCfg *config.Config

		mockey.Mock(gpudmanager.New).To(func(dataDir string) (*gpudmanager.Manager, error) {
			return &gpudmanager.Manager{}, nil
		}).Build()
		mockey.Mock((*gpudmanager.Manager).Start).To(func(_ *gpudmanager.Manager, _ context.Context) error {
			return nil
		}).Build()
		mockey.Mock(gpudserver.New).To(func(_ context.Context, _ log.AuditLogger, cfg *config.Config, _ *gpudmanager.Manager) (*gpudserver.Server, error) {
			receivedCfg = cfg
			return &gpudserver.Server{}, nil
		}).Build()
		mockey.Mock(gpudserver.HandleSignals).To(func(_ context.Context, _ context.CancelFunc, _ chan os.Signal, _ chan gpudserver.ServerStopper, _ func(context.Context) error) chan struct{} {
			done := make(chan struct{})
			close(done)
			return done
		}).Build()
		mockey.Mock(pkgsystemd.SystemctlExists).To(func() bool {
			return false
		}).Build()

		err := Command(ctx)
		require.NoError(t, err)
		require.NotNil(t, receivedCfg)
		assert.Equal(t, 7*time.Minute, receivedCfg.RetentionPeriod.Duration)
	})
}

func TestCommand_InvalidInfinibandJSON(t *testing.T) {
	tmpDir := t.TempDir()

	ctx := newTestCLIContext(t, cliFlagValues{
		stringFlags: map[string]string{
			"data-dir":                        tmpDir,
			"infiniband-expected-port-states": "{not-valid-json}",
		},
	})

	err := Command(ctx)
	require.Error(t, err)
}
