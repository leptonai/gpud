// Package up implements the "up" command.
package up

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/server"
	"github.com/leptonai/gpud/pkg/sqlite"
	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

var errEmptyToken = errors.New("token is empty")

func Command(cliContext *cli.Context) (retErr error) {
	logLevel := cliContext.String("log-level")
	logFile := cliContext.String("log-file")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	log.Logger.Debugw("starting up command")

	if err := osutil.RequireRoot(); err != nil {
		return err
	}

	// step 1.
	// perform "login" if and only if configured
	if cliContext.IsSet("token") || cliContext.String("token") != "" {
		log.Logger.Debugw("attempting control plane login")
		if lerr := performLogin(cliContext); lerr != nil {
			return lerr
		}
		log.Logger.Debugw("successfully logged in")
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
	if err := systemdInit(endpoint); err != nil {
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

func performLogin(cliContext *cli.Context) error {
	token := cliContext.String("token")
	if token == "" {
		fmt.Print("Please visit https://dashboard.lepton.ai/ under Settings/Tokens to fetch your token\nPlease enter your token:")
		if _, err := fmt.Scanln(&token); err != nil && err.Error() != "unexpected newline" {
			return fmt.Errorf("failed reading input: %w", err)
		}
	}
	if token == "" {
		return errEmptyToken
	}

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	log.Logger.Debugw("getting state file")
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}
	log.Logger.Debugw("successfully got state file")

	log.Logger.Debugw("opening state file for writing")
	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()
	log.Logger.Debugw("successfully opened state file for writing")

	log.Logger.Debugw("opening state file for reading")
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()
	log.Logger.Debugw("successfully opened state file for reading")

	log.Logger.Debugw("creating metadata table")
	if err := pkgmetadata.CreateTableMetadata(rootCtx, dbRW); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}
	log.Logger.Debugw("successfully created metadata table")

	log.Logger.Debugw("reading machine ID with fallback")
	prevMachineID, err := pkgmetadata.ReadMachineIDWithFallback(rootCtx, dbRW, dbRO)
	if err != nil {
		return err
	}
	log.Logger.Debugw("successfully read machine ID with fallback")

	if prevMachineID != "" {
		fmt.Printf("machine ID %s already assigned (skipping login)\n", prevMachineID)
		return nil
	}

	log.Logger.Debugw("creating nvml instance")
	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return fmt.Errorf("failed to create nvml instance: %w", err)
	}
	log.Logger.Debugw("successfully created nvml instance")
	defer func() {
		log.Logger.Debugw("shutting down nvml instance")
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Debugw("failed to shutdown nvml instance", "error", err)
		} else {
			log.Logger.Debugw("successfully shut down nvml instance")
		}
	}()

	machineID := cliContext.String("machine-id")
	nodeGroup := cliContext.String("node-group")

	var gpuCount string
	if cliContext.IsSet("gpu-count") {
		gpuCount = strconv.Itoa(cliContext.Int("gpu-count"))
	}

	loginCreatedAt := time.Now()
	log.Logger.Debugw("creating login request")
	req, err := pkgmachineinfo.CreateLoginRequest(token, machineID, nodeGroup, gpuCount, nvmlInstance)
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	log.Logger.Debugw("successfully created login request", "duration", time.Since(loginCreatedAt))

	publicIP := cliContext.String("public-ip")
	if publicIP != "" {
		req.Network.PublicIP = publicIP
	}

	privateIP := cliContext.String("private-ip")
	if privateIP != "" {
		req.Network.PrivateIP = privateIP
	}

	loginSentAt := time.Now()
	log.Logger.Debugw("sending login request")
	endpoint := cliContext.String("endpoint")
	loginResp, err := login.SendRequest(rootCtx, endpoint, *req)
	if err != nil {
		log.Logger.Debugw("failed to login", "error", err)
		if loginResp != nil {
			es := ""
			errorMessage := loginResp.Message
			if errorMessage == "" {
				// nolint:staticcheck // SA1019 This field is used for compatibility with older versions.
				errorMessage = loginResp.Error
			}
			if errorMessage != "" {
				es = fmt.Sprintf(", error: %s", errorMessage)
			}
			statusCode := loginResp.Code
			if statusCode == "" {
				// nolint:staticcheck // SA1019 This field is used for compatibility with older versions.
				statusCode = loginResp.Status
			}
			return fmt.Errorf("failed to login (reason: %s%s)", statusCode, es)
		}
		return err
	}
	log.Logger.Debugw("successfully sent login request", "duration", time.Since(loginSentAt))

	log.Logger.Debugw("recording endpoint")
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyEndpoint, endpoint); err != nil {
		return fmt.Errorf("failed to record endpoint: %w", err)
	}
	log.Logger.Debugw("successfully recorded endpoint")

	log.Logger.Debugw("recording machine ID")
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyMachineID, loginResp.MachineID); err != nil {
		return fmt.Errorf("failed to record machine ID: %w", err)
	}
	log.Logger.Debugw("successfully recorded machine ID")

	log.Logger.Debugw("recording session token")
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyToken, loginResp.Token); err != nil {
		return fmt.Errorf("failed to record session token: %w", err)
	}
	log.Logger.Debugw("successfully recorded session token")

	log.Logger.Debugw("recording public IP")
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyPublicIP, req.Network.PublicIP); err != nil {
		return fmt.Errorf("failed to record public IP: %w", err)
	}
	log.Logger.Debugw("successfully recorded public IP")

	log.Logger.Debugw("recording private IP")
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyPrivateIP, req.Network.PrivateIP); err != nil {
		return fmt.Errorf("failed to record private IP: %w", err)
	}
	log.Logger.Debugw("successfully recorded private IP")

	log.Logger.Debugw("getting fifo file")
	fifoFile, err := config.DefaultFifoFile()
	if err != nil {
		return fmt.Errorf("failed to get fifo file: %w", err)
	}
	log.Logger.Debugw("successfully got fifo file")

	log.Logger.Debugw("recording login success")
	if err := pkgmetadata.SetMetadata(rootCtx, dbRW, pkgmetadata.MetadataKeyControlPlaneLoginSuccess, fmt.Sprintf("%d", time.Now().Unix())); err != nil {
		return fmt.Errorf("failed to record login success: %w", err)
	}
	log.Logger.Debugw("successfully recorded login success")

	if serverRunning() {
		log.Logger.Debugw("server is running, writing token to fifo file")
		if err := server.WriteToken(loginResp.Token, fifoFile); err != nil {
			log.Logger.Debugw("failed to write token -- login before first gpud run/up", "error", err)
		} else {
			log.Logger.Debugw("successfully wrote token to fifo file")
		}
	}

	if len(loginResp.ValidationResults) > 0 {
		fmt.Printf("validation results:\n")
		invalids := 0
		for _, result := range loginResp.ValidationResults {
			if result.Valid {
				continue
			}
			invalids++
			fmt.Printf("%s %s: %s (%s)\n", cmdcommon.WarningSign, result.Name, result.Reason, result.Suggestion)
		}
		if invalids > 0 {
			fmt.Printf("please fix the above issues and try again\n")
		} else {
			fmt.Printf("all checks passed\n")
		}
	}

	fmt.Printf("%s successfully logged in and assigned machine id %s\n", cmdcommon.CheckMark, loginResp.MachineID)
	return nil
}

func systemdInit(endpoint string) error {
	if err := systemd.CreateDefaultEnvFile(endpoint); err != nil {
		return err
	}
	systemdUnitFileData := systemd.GPUdServiceUnitFileContents()
	return os.WriteFile(systemd.DefaultUnitFile, []byte(systemdUnitFileData), 0644)
}

func serverRunning() bool {
	if pkdsystemd.SystemctlExists() {
		log.Logger.Debugw("checking if gpud.service is active")
		active, err := pkdsystemd.IsActive("gpud.service")
		if err != nil {
			log.Logger.Debugw("failed to check if gpud.service is active", "error", err)
			return false
		}
		log.Logger.Debugw("successfully checked if gpud.service is active", "active", active)

		if active {
			return true
		}
	}
	return false
}
