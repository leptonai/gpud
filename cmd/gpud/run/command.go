// Package run implements the "run" command.
package run

import (
	"context"
	"encoding/json"
	"errors"
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
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	"github.com/leptonai/gpud/pkg/config"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	gpudserver "github.com/leptonai/gpud/pkg/server"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
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
	log.Logger = log.CreateLogger(zapLvl, logFile)

	log.Logger.Debugw("starting run command")

	dataDir, err := common.ResolveDataDir(cliContext)
	if err != nil {
		return err
	}

	// Parse db-in-memory early as it affects login behavior
	dbInMemory := cliContext.Bool("db-in-memory")

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

	// Note: login.Login() ALWAYS writes to the persistent state file (via dataDir),
	// regardless of --db-in-memory flag. The login package doesn't know about in-memory mode.
	// Only gpud run (via server.New) respects --db-in-memory and creates an in-memory database.
	if cliContext.IsSet("token") || controlPlaneLoginRegistrationToken != "" {
		log.Logger.Debugw("attempting control plane login")

		// Create login configuration from CLI context
		loginCtx, loginCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer loginCancel()

		loginCfg := login.LoginConfig{
			Token:     controlPlaneLoginRegistrationToken,
			Endpoint:  controlPlaneEndpoint,
			MachineID: machineIDForOverride,
			DataDir:   dataDir,

			GPUCount: gpuCountStr,
		}

		// on successful login, we persist the session token in the metadata for future re-use
		if lerr := login.Login(loginCtx, loginCfg); lerr != nil {
			return lerr
		}
		log.Logger.Infow("successfully logged in in gpud run")

		if err := recordLoginSuccessState(loginCtx, dataDir); err != nil {
			log.Logger.Warnw("failed to persist login success state", "error", err)
		}
	} else {
		log.Logger.Infow("no --token provided, skipping login")
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
	retentionPeriod := cliContext.Duration("retention-period")
	enableAutoUpdate := cliContext.Bool("enable-auto-update")
	autoUpdateExitCode := cliContext.Int("auto-update-exit-code")
	versionFile := cliContext.String("version-file")
	versionFileSet := cliContext.IsSet("version-file")
	pluginSpecsFile := cliContext.String("plugin-specs-file")
	skipSessionUpdateConfig := cliContext.Bool("skip-session-update-config")

	ibClassRootDir := cliContext.String("infiniband-class-root-dir")
	ibExcludeDevicesStr := cliContext.String("infiniband-exclude-devices")
	components := cliContext.String("components")

	infinibandExpectedPortStates := cliContext.String("infiniband-expected-port-states")
	nvlinkExpectedLinkStates := cliContext.String("nvlink-expected-link-states")
	nfsCheckerConfigs := cliContext.String("nfs-checker-configs")
	xidRebootThreshold := cliContext.Int("xid-reboot-threshold")

	if len(infinibandExpectedPortStates) > 0 {
		var expectedPortStates componentsnvidiainfinibanditypes.ExpectedPortStates
		if err := json.Unmarshal([]byte(infinibandExpectedPortStates), &expectedPortStates); err != nil {
			return err
		}
		componentsinfiniband.SetDefaultExpectedPortStates(expectedPortStates)

		log.Logger.Infow("set infiniband expected port states", "infinibandExpectedPortStates", infinibandExpectedPortStates)
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

	if cliContext.IsSet("xid-reboot-threshold") {
		if xidRebootThreshold > 0 {
			componentsxid.SetDefaultRebootThreshold(componentsxid.RebootThreshold{
				Threshold: xidRebootThreshold,
			})
			log.Logger.Infow("set xid reboot threshold", "xidRebootThreshold", xidRebootThreshold)
		} else {
			log.Logger.Warnw("ignoring xid reboot threshold override, value must be positive", "xidRebootThreshold", xidRebootThreshold)
		}
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
		}),
	}

	// When --db-in-memory is enabled, read session credentials from the persistent state file
	// and pass them to the config so the server can seed them into the in-memory database.
	// This works because login.Login() ALWAYS writes to persistent file (via dataDir),
	// and only the server respects --db-in-memory for its runtime database.
	//
	// IMPORTANT: The endpoint MUST also be seeded because the server reads the endpoint
	// from the metadata DB (not from config) for session keepalive.
	if dbInMemory {
		readCtx, readCancel := context.WithTimeout(context.Background(), 10*time.Second)
		sessionToken, assignedMachineID, endpoint, readErr := readSessionCredentialsFromPersistentFile(readCtx, dataDir)
		readCancel()

		if readErr != nil {
			if errors.Is(readErr, errStateFileNotFound) {
				// This is expected on fresh installs where login hasn't been performed yet.
				// Session keepalive will not work until login is completed.
				log.Logger.Infow(
					"persistent state file not found for db-in-memory mode (fresh install)",
					"dataDir", dataDir,
				)
			} else {
				// Other errors (corrupted file, permission issues, etc.) are more concerning.
				log.Logger.Warnw(
					"failed to read session credentials from persistent file for db-in-memory mode",
					"error", readErr,
				)
			}
			// Continue without session credentials - server will need to handle authentication

		} else {
			// The persistent state file is the source-of-truth for session credentials.
			// However, if the endpoint isn't present there (e.g., old/partial state),
			// fall back to the CLI flag (systemd env file).
			if endpoint == "" && controlPlaneEndpoint != "" {
				endpoint = controlPlaneEndpoint
			}

			if sessionToken != "" && assignedMachineID != "" && endpoint != "" {
				log.Logger.Infow(
					"read session credentials from persistent file for db-in-memory mode",
					"machineID", assignedMachineID,
					"endpoint", endpoint,
				)
				configOpts = append(configOpts,
					config.WithSessionToken(sessionToken),
					config.WithSessionMachineID(assignedMachineID),
					config.WithSessionEndpoint(endpoint),
				)
			} else {
				// Credentials were read but are incomplete - this may indicate a partial state
				log.Logger.Warnw(
					"db-in-memory mode enabled but session credentials are incomplete",
					"hasToken", sessionToken != "",
					"hasMachineID", assignedMachineID != "",
					"hasEndpoint", endpoint != "",
				)
			}
		}
	}

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
	if retentionPeriod > 0 {
		cfg.RetentionPeriod = metav1.Duration{Duration: retentionPeriod}
	}

	cfg.CompactPeriod = config.DefaultCompactPeriod

	cfg.EnableAutoUpdate = enableAutoUpdate
	cfg.AutoUpdateExitCode = autoUpdateExitCode
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

		dbRW, err := pkgsqlite.Open(cfg.State)
		if err != nil {
			return fmt.Errorf("failed to open state for metadata overrides: %w", err)
		}
		defer func() {
			_ = dbRW.Close()
		}()

		if err := pkgmetadata.CreateTableMetadata(mctx, dbRW); err != nil {
			return fmt.Errorf("failed to ensure metadata table: %w", err)
		}

		if controlPlaneEndpoint != "" {
			if err := pkgmetadata.SetMetadata(mctx, dbRW, pkgmetadata.MetadataKeyEndpoint, controlPlaneEndpoint); err != nil {
				return fmt.Errorf("failed to set endpoint metadata: %w", err)
			}
			log.Logger.Infow("overriding endpoint from flag", "endpoint", controlPlaneEndpoint)
		}

		// DO NOT overwrite "pkgmetadata.MetadataKeyToken"
		// because successful login operation will persist the session token in the metadata
		// NOT the registration token

		if machineIDForOverride != "" {
			if err := pkgmetadata.SetMetadata(mctx, dbRW, pkgmetadata.MetadataKeyMachineID, machineIDForOverride); err != nil {
				return fmt.Errorf("failed to set machine-id metadata: %w", err)
			}
			log.Logger.Infow("overriding machine id from flag", "machineID", machineIDForOverride)
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

func recordLoginSuccessState(ctx context.Context, dataDir string) error {
	resolvedDataDir, err := config.ResolveDataDir(dataDir)
	if err != nil {
		return fmt.Errorf("failed to resolve data dir for state: %w", err)
	}

	stateFile := config.StateFilePath(resolvedDataDir)

	dbRW, err := pkgsqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer func() {
		_ = dbRW.Close()
	}()

	if err := sessionstates.CreateTable(ctx, dbRW); err != nil {
		return fmt.Errorf("failed to create session states table: %w", err)
	}

	if err := sessionstates.Insert(ctx, dbRW, time.Now().Unix(), true, "Session connected successfully"); err != nil {
		return fmt.Errorf("failed to record login success state: %w", err)
	}

	return nil
}

// errStateFileNotFound is returned when the persistent state file doesn't exist.
// This is expected on fresh installs where login hasn't been performed yet.
var errStateFileNotFound = fmt.Errorf("state file not found")

// readSessionCredentialsFromPersistentFile reads the session token, assigned machine ID,
// and endpoint from the persistent state file. This is used when --db-in-memory is enabled
// to seed the session credentials into the in-memory database.
//
// Note: login.Login() ALWAYS writes to persistent file (via dataDir), regardless of
// --db-in-memory flag. Only the server respects --db-in-memory for its runtime database.
//
// The server reads the endpoint from metadata DB (not from config), so the endpoint
// MUST be seeded into the in-memory database for session keepalive to work.
//
// Returns errStateFileNotFound if the state file doesn't exist (fresh install).
func readSessionCredentialsFromPersistentFile(ctx context.Context, dataDir string) (sessionToken string, assignedMachineID string, endpoint string, err error) {
	resolvedDataDir, err := config.ResolveDataDir(dataDir)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to resolve data dir: %w", err)
	}

	stateFile := config.StateFilePath(resolvedDataDir)

	// Check if state file exists before trying to open it
	if _, statErr := os.Stat(stateFile); os.IsNotExist(statErr) {
		return "", "", "", errStateFileNotFound
	}

	dbRO, err := pkgsqlite.Open(stateFile, pkgsqlite.WithReadOnly(true))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to open state file: %w", err)
	}
	defer func() {
		_ = dbRO.Close()
	}()

	sessionToken, err = pkgmetadata.ReadToken(ctx, dbRO)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read session token: %w", err)
	}

	assignedMachineID, err = pkgmetadata.ReadMachineID(ctx, dbRO)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read machine ID: %w", err)
	}

	endpoint, err = pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyEndpoint)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read endpoint: %w", err)
	}

	return sessionToken, assignedMachineID, endpoint, nil
}
