// Package run implements the "run" command.
package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/cmd/gpud/common"
	gpudcomponents "github.com/leptonai/gpud/components"
	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsinfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsnvidiainfinibanditypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	componentsnvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	componentssxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	componentstemperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	"github.com/leptonai/gpud/pkg/config"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	gpudserver "github.com/leptonai/gpud/pkg/server"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
	pkgsystemd "github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/version"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	logFile := cliContext.String("log-file")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.CreateLogger(zapLvl, logFile))

	log.Logger.Debugw("starting run command")

	dataDir, err := common.ResolveDataDir(cliContext)
	if err != nil {
		return err
	}

	// Parse db-in-memory early as it affects login behavior
	dbInMemory := cliContext.Bool("db-in-memory")

	metricsRetentionPeriod, eventsRetentionPeriod := parseRetentionPeriods(cliContext)

	gpuCount := cliContext.Int("gpu-count")
	gpuCountStr := ""
	if gpuCount > 0 {
		gpuCountStr = fmt.Sprintf("%d", gpuCount)
		componentsnvidiagpucounts.SetDefaultExpectedGPUCounts(componentsnvidiagpucounts.ExpectedGPUCounts{
			Count: gpuCount,
		})
		log.Logger.Infow("set gpu count", "gpuCount", gpuCount)
	}

	// step 1.
	// perform "login" if and only if configured
	// Optional overrides for control plane connectivity
	controlPlaneEndpoint := cliContext.String("endpoint")

	// Represents the machine registration login token.
	// This is the token that GPUd sends to the control plane to register the machine.
	// This is NOT the token that GPUd uses for session authentication.
	controlPlaneLoginRegistrationToken := cliContext.String("token")

	machineIDForOverride := cliContext.String("machine-id")
	shouldOverwriteMachineID := cliContext.Bool("machine-id-overwrite")
	refreshSessionToken := cliContext.Bool("refresh-session-token")
	controlPlaneLoginSucceeded := false

	// Note: login.Login() ALWAYS writes to the persistent state file (via dataDir),
	// regardless of --db-in-memory flag. The login package doesn't know about in-memory mode.
	// Only gpud run (via server.New) respects --db-in-memory and creates an in-memory database.
	if cliContext.IsSet("token") || controlPlaneLoginRegistrationToken != "" {
		log.Logger.Debugw("attempting control plane login")

		// Create login configuration from CLI context
		loginCtx, loginCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer loginCancel()

		loginCfg := login.LoginConfig{
			Token:               controlPlaneLoginRegistrationToken,
			Endpoint:            controlPlaneEndpoint,
			MachineID:           machineIDForOverride,
			MachineIDOverwrite:  shouldOverwriteMachineID,
			RefreshSessionToken: refreshSessionToken,
			DataDir:             dataDir,

			GPUCount: gpuCountStr,
		}

		// on successful login, we persist the session token in the metadata for future re-use
		if lerr := login.Login(loginCtx, loginCfg); lerr != nil {
			return lerr
		}
		controlPlaneLoginSucceeded = true
		log.Logger.Infow("successfully logged in in gpud run")

		if err := recordLoginSuccessState(loginCtx, dataDir); err != nil {
			log.Logger.Warnw("failed to persist login success state", "error", err)
		}
	} else {
		log.Logger.Infow("no gpud run --token provided, skipping login")
	}

	if runtime.GOOS != "linux" {
		fmt.Printf("gpud run on %q not supported\n", runtime.GOOS)
		os.Exit(1)
	}

	if zapLvl.Level() > zap.DebugLevel { // e.g., info, warn, error
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	listenAddress := cliContext.String("listen-address")
	pprof := cliContext.Bool("pprof")

	enableAutoUpdate := cliContext.Bool("enable-auto-update")
	autoUpdateExitCode := cliContext.Int("auto-update-exit-code")
	rebootCommands := cliContext.String("reboot-commands")
	findmntCommands := cliContext.String("findmnt-commands")
	lsblkCommands := cliContext.String("lsblk-commands")
	blockdevUsageCommands := cliContext.String("blockdev-usage-commands")
	containerdServiceActiveCommands := cliContext.String("containerd-service-active-commands")
	versionFile := cliContext.String("version-file")
	versionFileSet := cliContext.IsSet("version-file")
	pluginSpecsFile := cliContext.String("plugin-specs-file")
	skipSessionUpdateConfig := cliContext.Bool("skip-session-update-config")

	ibClassRootDir := cliContext.String("infiniband-class-root-dir")
	ibExcludeDevicesStr := cliContext.String("infiniband-exclude-devices")
	components := cliContext.String("components")

	infinibandExpectedPortStates := cliContext.String("infiniband-expected-port-states")
	infinibandFlapAutoClearWindow := cliContext.Duration("infiniband-flap-auto-clear-window")
	nvlinkExpectedLinkStates := cliContext.String("nvlink-expected-link-states")
	nfsCheckerConfigs := cliContext.String("nfs-checker-configs")
	xidRebootThreshold := cliContext.Int("xid-reboot-threshold")
	xidThresholds := cliContext.String("xid-thresholds")
	sxidThresholds := cliContext.String("sxid-thresholds")
	temperatureMarginThresholdCelsius := cliContext.Int("threshold-celsius-slowdown-margin")

	if len(infinibandExpectedPortStates) > 0 {
		var expectedPortStates componentsnvidiainfinibanditypes.ExpectedPortStates
		if err := json.Unmarshal([]byte(infinibandExpectedPortStates), &expectedPortStates); err != nil {
			return err
		}
		componentsinfiniband.SetDefaultExpectedPortStates(expectedPortStates)

		log.Logger.Infow("set infiniband expected port states", "infinibandExpectedPortStates", infinibandExpectedPortStates)
	}

	if infinibandFlapAutoClearWindow > 0 {
		componentsinfiniband.SetDefaultFlapAutoClearWindow(infinibandFlapAutoClearWindow)
		log.Logger.Infow("set infiniband flap auto-clear window", "infinibandFlapAutoClearWindow", infinibandFlapAutoClearWindow)
	}

	if len(nvlinkExpectedLinkStates) > 0 {
		var expectedLinkStates componentsnvlink.ExpectedLinkStates
		if err := json.Unmarshal([]byte(nvlinkExpectedLinkStates), &expectedLinkStates); err != nil {
			return err
		}
		componentsnvlink.SetDefaultExpectedLinkStates(expectedLinkStates)

		log.Logger.Infow("set nvlink expected link states", "nvlinkExpectedLinkStates", nvlinkExpectedLinkStates)
	}

	if len(nfsCheckerConfigs) > 0 {
		groupConfigs := make(pkgnfschecker.Configs, 0)
		if err := json.Unmarshal([]byte(nfsCheckerConfigs), &groupConfigs); err != nil {
			return err
		}
		componentsnfs.SetDefaultConfigs(groupConfigs)

		log.Logger.Infow("set nfs checker group configs", "groupConfigs", groupConfigs)
	}

	xidThresholdsChanged := false
	if cliContext.IsSet("xid-reboot-threshold") {
		if xidRebootThreshold > 0 {
			componentsxid.SetDefaultRebootThreshold(xidRebootThreshold)
			xidThresholdsChanged = true
		} else {
			log.Logger.Warnw("ignoring xid reboot threshold override, value must be positive", "xidRebootThreshold", xidRebootThreshold)
		}
	}
	if strings.TrimSpace(xidThresholds) != "" {
		thresholds, err := parseXIDThresholds(xidThresholds)
		if err != nil {
			return err
		}
		componentsxid.SetDefaultThresholds(thresholds)
		xidThresholdsChanged = true
	}
	if xidThresholdsChanged {
		xidThresholdConfig := componentsxid.GetDefaultThresholds()
		log.Logger.Infow(
			"set xid thresholds",
			"xidDefaultRebootThreshold",
			componentsxid.GetDefaultRebootThreshold(),
			"xidOverrides",
			xidThresholdConfig.Overrides,
		)
	}

	if strings.TrimSpace(sxidThresholds) != "" {
		thresholds, err := parseSXIDThresholds(sxidThresholds)
		if err != nil {
			return err
		}
		componentssxid.SetDefaultThresholds(thresholds)
		log.Logger.Infow("set sxid thresholds", "sxidOverrides", thresholds.Overrides)
	}

	if eventsRetentionPeriod > 0 && !cliContext.IsSet("xid-lookback-period") {
		componentsxid.SetLookbackPeriod(eventsRetentionPeriod)
		log.Logger.Infow("set xid lookback period from events retention period", "xidLookbackPeriod", eventsRetentionPeriod)
	}

	if cliContext.IsSet("xid-lookback-period") {
		componentsxid.SetLookbackPeriod(cliContext.Duration("xid-lookback-period"))
		log.Logger.Infow("set xid lookback period", "xidLookbackPeriod", cliContext.Duration("xid-lookback-period"))
	}

	if eventsRetentionPeriod > 0 && !cliContext.IsSet("sxid-lookback-period") {
		componentssxid.SetLookbackPeriod(eventsRetentionPeriod)
		log.Logger.Infow("set sxid lookback period from events retention period", "sxidLookbackPeriod", eventsRetentionPeriod)
	}

	if cliContext.IsSet("sxid-lookback-period") {
		componentssxid.SetLookbackPeriod(cliContext.Duration("sxid-lookback-period"))
		log.Logger.Infow("set sxid lookback period", "sxidLookbackPeriod", cliContext.Duration("sxid-lookback-period"))
	}

	if cliContext.IsSet("threshold-celsius-slowdown-margin") {
		componentstemperature.SetDefaultMarginThreshold(componentstemperature.Thresholds{
			CelsiusSlowdownMargin: int32(temperatureMarginThresholdCelsius),
		})
		log.Logger.Infow("set temperature margin threshold", "degraded_celsius", temperatureMarginThresholdCelsius)
	}

	gpuUUIDsWithRowRemappingPendingRaw := cliContext.String("gpu-uuids-with-row-remapping-pending")
	gpuUUIDsWithRowRemappingPending := common.ParseGPUUUIDs(gpuUUIDsWithRowRemappingPendingRaw)

	gpuUUIDsWithRowRemappingFailedRaw := cliContext.String("gpu-uuids-with-row-remapping-failed")
	gpuUUIDsWithRowRemappingFailed := common.ParseGPUUUIDs(gpuUUIDsWithRowRemappingFailedRaw)

	gpuUUIDsWithHWSlowdownRaw := cliContext.String("gpu-uuids-with-hw-slowdown")
	gpuUUIDsWithHWSlowdown := common.ParseGPUUUIDs(gpuUUIDsWithHWSlowdownRaw)

	gpuUUIDsWithHWSlowdownThermalRaw := cliContext.String("gpu-uuids-with-hw-slowdown-thermal")
	gpuUUIDsWithHWSlowdownThermal := common.ParseGPUUUIDs(gpuUUIDsWithHWSlowdownThermalRaw)

	gpuUUIDsWithHWSlowdownPowerBrakeRaw := cliContext.String("gpu-uuids-with-hw-slowdown-power-brake")
	gpuUUIDsWithHWSlowdownPowerBrake := common.ParseGPUUUIDs(gpuUUIDsWithHWSlowdownPowerBrakeRaw)

	gpuUUIDsWithGPULostRaw := cliContext.String("gpu-uuids-with-gpu-lost")
	gpuUUIDsWithGPULost := common.ParseGPUUUIDs(gpuUUIDsWithGPULostRaw)

	gpuUUIDsWithGPURequiresResetRaw := cliContext.String("gpu-uuids-with-gpu-requires-reset")
	gpuUUIDsWithGPURequiresReset := common.ParseGPUUUIDs(gpuUUIDsWithGPURequiresResetRaw)

	// NOTE: This flag only takes effect on multi-GPU NVSwitch systems (H100-SXM, H200-SXM, GB200).
	// It is IGNORED on: PCIe variants (H100-PCIe, H200-PCIe), single-GPU systems, non-Hopper GPUs.
	// See: components/accelerator/nvidia/fabric-manager/component.go for detailed conditions.
	// Use --gpu-product-name to override the product name and enable fabric state checking on PCIe systems.
	gpuUUIDsWithFabricStateHealthSummaryUnhealthyRaw := cliContext.String("gpu-uuids-with-fabric-state-health-summary-unhealthy")
	gpuUUIDsWithFabricStateHealthSummaryUnhealthy := common.ParseGPUUUIDs(gpuUUIDsWithFabricStateHealthSummaryUnhealthyRaw)

	// GPU product name override for testing - allows simulating different GPU types
	// (e.g., set "H100-SXM" on H100-PCIe to enable fabric state failure injection testing)
	gpuProductNameOverride := cliContext.String("gpu-product-name")

	// NVML device enumeration error injection for testing
	// When enabled, Device().GetDevices() returns an error simulating Xid 79 or similar failures
	nvmlDeviceGetDevicesError := cliContext.Bool("nvml-device-get-devices-error")

	// Containerd socket missing error injection for testing
	// When enabled, the containerd component will report the socket file as missing
	containerdSocketMissing := cliContext.Bool("containerd-socket-missing")

	ibExcludedDevices := parseInfinibandExcludeDevices(ibExcludeDevicesStr)
	if len(ibExcludedDevices) > 0 {
		log.Logger.Infow("excluding infiniband devices from monitoring", "devices", ibExcludedDevices)
	}

	configOpts := []config.OpOption{
		config.WithDataDir(dataDir),
		config.WithInfinibandClassRootDir(ibClassRootDir),
		config.WithDBInMemory(dbInMemory),
		config.WithExcludedInfinibandDevices(ibExcludedDevices),
		config.WithFailureInjector(&gpudcomponents.FailureInjector{
			GPUUUIDsWithRowRemappingPending:               gpuUUIDsWithRowRemappingPending,
			GPUUUIDsWithRowRemappingFailed:                gpuUUIDsWithRowRemappingFailed,
			GPUUUIDsWithHWSlowdown:                        gpuUUIDsWithHWSlowdown,
			GPUUUIDsWithHWSlowdownThermal:                 gpuUUIDsWithHWSlowdownThermal,
			GPUUUIDsWithHWSlowdownPowerBrake:              gpuUUIDsWithHWSlowdownPowerBrake,
			GPUUUIDsWithGPULost:                           gpuUUIDsWithGPULost,
			GPUUUIDsWithGPURequiresReset:                  gpuUUIDsWithGPURequiresReset,
			GPUUUIDsWithFabricStateHealthSummaryUnhealthy: gpuUUIDsWithFabricStateHealthSummaryUnhealthy,
			GPUProductNameOverride:                        gpuProductNameOverride,
			NVMLDeviceGetDevicesError:                     nvmlDeviceGetDevicesError,
			ContainerdSocketMissing:                       containerdSocketMissing,
		}),
	}

	configOpts = append(configOpts, getSessionCredentialsOptions(dbInMemory, dataDir, controlPlaneEndpoint)...)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cfg, err := config.DefaultConfig(ctx, configOpts...)
	cancel()
	if err != nil {
		return err
	}

	if listenAddress != "" {
		cfg.Address = listenAddress
	}
	if pprof {
		cfg.Pprof = true
	}
	if metricsRetentionPeriod > 0 {
		cfg.MetricsRetentionPeriod = metav1.Duration{Duration: metricsRetentionPeriod}
	}
	if eventsRetentionPeriod > 0 {
		cfg.EventsRetentionPeriod = metav1.Duration{Duration: eventsRetentionPeriod}
	}

	cfg.CompactPeriod = config.DefaultCompactPeriod

	cfg.EnableAutoUpdate = enableAutoUpdate
	cfg.AutoUpdateExitCode = autoUpdateExitCode
	cfg.RebootCommands = rebootCommands
	cfg.FindmntCommands = findmntCommands
	cfg.LsblkCommands = lsblkCommands
	cfg.BlockdevUsageCommands = blockdevUsageCommands
	cfg.ContainerdServiceActiveCommands = containerdServiceActiveCommands
	if !versionFileSet {
		versionFile = config.VersionFilePath(cfg.DataDir)
	}
	cfg.VersionFile = versionFile

	cfg.PluginSpecsFile = pluginSpecsFile
	cfg.SkipSessionUpdateConfig = skipSessionUpdateConfig

	if components != "" {
		cfg.Components = strings.Split(components, ",")
	}

	auditLogger := log.NewNopAuditLogger()
	if logFile != "" {
		logAuditFile := log.CreateAuditLogFilepath(logFile)
		auditLogger = log.NewAuditLogger(logAuditFile)
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	// Persist overrides to metadata for subsequent sessions.
	// Skip when using in-memory database as data won't persist across restarts.
	if !cfg.DBInMemory && (controlPlaneEndpoint != "" || machineIDForOverride != "") {
		mctx, mcancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer mcancel()

		if err := persistMetadataOverrides(mctx, cfg.State, controlPlaneEndpoint, machineIDForOverride, shouldOverwriteMachineID, controlPlaneLoginSucceeded); err != nil {
			return err
		}
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	start := time.Now()

	signals := make(chan os.Signal, 2048)
	serverC := make(chan gpudserver.ServerStopper, 1)

	log.Logger.Infof("starting gpud %v", version.Version)

	done := gpudserver.HandleSignals(rootCtx, rootCancel, signals, serverC, func(ctx context.Context) error {
		if pkgsystemd.SystemctlExists() {
			if err := pkgsystemd.NotifyStopping(ctx); err != nil {
				log.Logger.Errorw("notify stopping failed")
			}
		}
		return nil
	})

	// start the signal handler as soon as we can to make sure that
	// we don't miss any signals during boot
	signal.Notify(signals, gpudserver.DefaultSignalsToHandle...)
	m, err := gpudmanager.New(cfg.DataDir)
	if err != nil {
		return err
	}
	if err := m.Start(rootCtx); err != nil {
		return err
	}

	server, err := gpudserver.New(rootCtx, auditLogger, cfg, m)
	if err != nil {
		return err
	}
	serverC <- server

	if pkgsystemd.SystemctlExists() {
		if err := pkgsystemd.NotifyReady(rootCtx); err != nil {
			log.Logger.Warnw("notify ready failed")
		}
	} else {
		log.Logger.Debugw("skipped sd notify as systemd is not available")
	}

	log.Logger.Infow("successfully booted", "tookSeconds", time.Since(start).Seconds())
	<-done

	return nil
}

func validateMachineIDOverride(prevMachineID, requestedMachineID string, overwrite, controlPlaneLoginSucceeded bool) error {
	if prevMachineID == "" || prevMachineID == requestedMachineID {
		return nil
	}
	if !overwrite {
		return fmt.Errorf("persisted machine ID %q differs from --machine-id %q; pass --machine-id-overwrite to replace it", prevMachineID, requestedMachineID)
	}
	if !controlPlaneLoginSucceeded {
		return fmt.Errorf("cannot overwrite persisted machine ID %q with --machine-id %q without a successful control-plane login; pass --token so gpud can check in to the requested machine", prevMachineID, requestedMachineID)
	}
	return nil
}

func persistMetadataOverrides(ctx context.Context, stateFile, controlPlaneEndpoint, machineIDForOverride string, machineIDOverwrite, controlPlaneLoginSucceeded bool) error {
	dbRW, err := pkgsqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state for metadata overrides: %w", err)
	}
	defer func() {
		_ = dbRW.Close()
	}()

	if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
		return fmt.Errorf("failed to ensure metadata table: %w", err)
	}

	if controlPlaneEndpoint != "" {
		if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyEndpoint, controlPlaneEndpoint); err != nil {
			return fmt.Errorf("failed to set endpoint metadata: %w", err)
		}
		log.Logger.Infow("overriding endpoint from flag", "endpoint", controlPlaneEndpoint)
	}

	if machineIDForOverride == "" {
		return nil
	}

	prevMachineID, err := pkgmetadata.ReadMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID)
	if err != nil {
		return fmt.Errorf("failed to read persisted machine-id: %w", err)
	}
	if err := validateMachineIDOverride(prevMachineID, machineIDForOverride, machineIDOverwrite, controlPlaneLoginSucceeded); err != nil {
		return err
	}

	// A successful control-plane login persists the machine ID returned by the
	// control plane. Do not overwrite that authoritative result with the CLI
	// value. The no-login path keeps the historical behavior of recording an
	// initial machine-id override locally.
	if prevMachineID == machineIDForOverride || controlPlaneLoginSucceeded {
		return nil
	}

	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, machineIDForOverride); err != nil {
		return fmt.Errorf("failed to set machine-id metadata: %w", err)
	}
	log.Logger.Infow("overriding machine id from flag", "machineID", machineIDForOverride)
	return nil
}

func parseRetentionPeriods(cliContext *cli.Context) (metricsRetentionPeriod, eventsRetentionPeriod time.Duration) {
	metricsRetentionPeriod = cliContext.Duration("metrics-retention-period")
	deprecatedRetentionPeriod := cliContext.Duration("retention-period")

	// Treat non-positive values as unset for backward compatibility.
	//
	// Precedence:
	// 1) --metrics-retention-period
	// 2) deprecated --retention-period
	// 3) config default
	if metricsRetentionPeriod <= 0 {
		if deprecatedRetentionPeriod > 0 {
			metricsRetentionPeriod = deprecatedRetentionPeriod
		} else {
			metricsRetentionPeriod = config.DefaultMetricsRetentionPeriod.Duration
		}
	}

	eventsRetentionPeriod = cliContext.Duration("events-retention-period")
	if eventsRetentionPeriod <= 0 {
		eventsRetentionPeriod = config.DefaultEventsRetentionPeriod.Duration
	}

	return metricsRetentionPeriod, eventsRetentionPeriod
}

type thresholdOverrideJSON struct {
	RebootThreshold int `json:"rebootThreshold"`
}

type thresholdsJSON struct {
	Overrides map[int]thresholdOverrideJSON `json:"overrides"`
}

func parseXIDThresholds(raw string) (componentsxid.Thresholds, error) {
	thresholds, err := parseThresholds(raw, "xid thresholds")
	if err != nil {
		return componentsxid.Thresholds{}, err
	}

	ret := make(map[int]componentsxid.ThresholdOverride, len(thresholds))
	for xid, threshold := range thresholds {
		ret[xid] = componentsxid.ThresholdOverride{
			RebootThreshold: threshold.RebootThreshold,
		}
	}
	return componentsxid.Thresholds{Overrides: ret}, nil
}

func parseSXIDThresholds(raw string) (componentssxid.Thresholds, error) {
	thresholds, err := parseThresholds(raw, "sxid thresholds")
	if err != nil {
		return componentssxid.Thresholds{}, err
	}

	ret := make(map[int]componentssxid.ThresholdOverride, len(thresholds))
	for sxid, threshold := range thresholds {
		ret[sxid] = componentssxid.ThresholdOverride{
			RebootThreshold: threshold.RebootThreshold,
		}
	}
	return componentssxid.Thresholds{Overrides: ret}, nil
}

func parseThresholds(raw string, name string) (map[int]thresholdOverrideJSON, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}
	if fields == nil {
		return nil, fmt.Errorf("invalid %s: expected JSON object", name)
	}
	for field := range fields {
		if field != "overrides" {
			return nil, fmt.Errorf("invalid %s: unknown field %q", name, field)
		}
	}

	var thresholds thresholdsJSON
	if err := json.Unmarshal([]byte(raw), &thresholds); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", name, err)
	}

	for id, threshold := range thresholds.Overrides {
		if id < 0 {
			return nil, fmt.Errorf("invalid %s for %d: event ID must be non-negative", name, id)
		}
		if threshold.RebootThreshold <= 0 {
			return nil, fmt.Errorf("invalid %s for %d: rebootThreshold must be positive", name, id)
		}
	}
	return thresholds.Overrides, nil
}

func parseInfinibandExcludeDevices(s string) []string {
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	devices := make([]string, 0, len(parts))
	for _, d := range parts {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		devices = append(devices, d)
	}
	if len(devices) == 0 {
		return nil
	}
	return devices
}
