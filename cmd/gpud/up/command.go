// Package up implements the "up" command.
package up

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	"github.com/leptonai/gpud/pkg/osutil"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

func Command(cliContext *cli.Context) (retErr error) {
	logLevel := cliContext.String("log-level")
	logFile := cliContext.String("log-file")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	log.Logger.Debugw("starting up command")

	dataDir, err := common.ResolveDataDir(cliContext)
	if err != nil {
		return err
	}

	if err := osutil.RequireRoot(); err != nil {
		return err
	}

	gpuCount := cliContext.Int("gpu-count")
	gpuCountStr := ""
	if gpuCount > 0 {
		gpuCountStr = fmt.Sprintf("%d", gpuCount)
	}

	// step 1.
	// perform "login" if and only if configured
	if cliContext.IsSet("token") || cliContext.String("token") != "" {
		log.Logger.Debugw("attempting control plane login")

		// Create login configuration from CLI context
		loginCtx, loginCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer loginCancel()

		loginCfg := login.LoginConfig{
			Token:     cliContext.String("token"),
			Endpoint:  cliContext.String("endpoint"),
			MachineID: cliContext.String("machine-id"),
			NodeGroup: cliContext.String("node-group"),
			DataDir:   dataDir,

			GPUCount: gpuCountStr,

			PublicIP:  cliContext.String("public-ip"),
			PrivateIP: cliContext.String("private-ip"),
		}

		if lerr := login.Login(loginCtx, loginCfg); lerr != nil {
			return lerr
		}
		log.Logger.Debugw("successfully logged in")

		if err := recordLoginSuccessState(loginCtx, dataDir); err != nil {
			log.Logger.Warnw("failed to persist login success state", "error", err)
		}
	} else {
		log.Logger.Infow("no --token provided, skipping login")
	}

	// step 2.
	// perform "run" to start the daemon in systemd service
	bin, err := os.Executable()
	if err != nil {
		return err
	}

	log.Logger.Debugw("checking if systemd exists")
	if !pkdsystemd.SystemctlExists() {
		return fmt.Errorf("requires systemd, to run without systemd, '%s run'", bin)
	}
	log.Logger.Debugw("systemd exists")

	log.Logger.Debugw("checking if gpud binary exists")
	if !systemd.DefaultBinExists() {
		return fmt.Errorf("gpud binary not found at %s (you may run 'cp %s %s' to fix the installation)", systemd.DefaultBinPath, bin, systemd.DefaultBinPath)
	}
	log.Logger.Debugw("gpud binary exists")

	log.Logger.Debugw("starting systemd init")
	endpoint := cliContext.String("endpoint")
	dbInMemory := cliContext.Bool("db-in-memory")
	if err := systemdInit(endpoint, dataDir, dbInMemory); err != nil {
		return err
	}
	log.Logger.Debugw("successfully started systemd init")

	log.Logger.Debugw("enabling systemd unit")
	if err := pkgupdate.EnableGPUdSystemdUnit(); err != nil {
		return err
	}
	log.Logger.Debugw("successfully enabled systemd unit")

	log.Logger.Debugw("restarting systemd unit")
	if err := pkgupdate.RestartGPUdSystemdUnit(); err != nil {
		return err
	}
	log.Logger.Debugw("successfully restarted systemd unit")

	log.Logger.Debugw("successfully started gpud (run 'gpud status' for checking status)")
	return nil
}

func recordLoginSuccessState(ctx context.Context, dataDir string) error {
	resolvedDataDir, err := config.ResolveDataDir(dataDir)
	if err != nil {
		return fmt.Errorf("failed to resolve data dir: %w", err)
	}

	stateFile := config.StateFilePath(resolvedDataDir)

	dbRW, err := sqlite.Open(stateFile)
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

func systemdInit(endpoint string, dataDir string, dbInMemory bool) error {
	if err := systemd.CreateDefaultEnvFile(endpoint, dataDir, dbInMemory); err != nil {
		return err
	}
	systemdUnitFileData := systemd.GPUdServiceUnitFileContents()
	return os.WriteFile(systemd.DefaultUnitFile, []byte(systemdUnitFileData), 0644)
}
