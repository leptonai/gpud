package login

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/config"
	gpudstate "github.com/leptonai/gpud/pkg/gpud-state"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/server"
	"github.com/leptonai/gpud/pkg/sqlite"
)

var (
	ErrEmptyToken = errors.New("token is empty")
)

func Command(cliContext *cli.Context) error {
	if err := osutil.RequireRoot(); err != nil {
		return err
	}

	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	token := cliContext.String("token")
	if token == "" {
		fmt.Print("Please visit https://dashboard.lepton.ai/ under Settings/Tokens to fetch your token\nPlease enter your token:")
		if _, err := fmt.Scanln(&token); err != nil && err.Error() != "unexpected newline" {
			return fmt.Errorf("failed reading input: %w", err)
		}
	}
	if token == "" {
		return ErrEmptyToken
	}

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()

	// in case the table has not been created
	if err := gpudstate.CreateTableMetadata(rootCtx, dbRW); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	prevMachineID, err := gpudstate.ReadMachineIDWithFallback(rootCtx, dbRW, dbRO)
	if err != nil {
		return err
	}
	if prevMachineID != "" {
		fmt.Printf("machine ID %s already assigned (skipping login)\n", prevMachineID)
		return nil
	}

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return fmt.Errorf("failed to create nvml instance: %w", err)
	}
	defer func() {
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Debugw("failed to shutdown nvml instance", "error", err)
		}
	}()

	// previous/existing machine ID is not found (can be empty)
	// if specified, the control plane will validate the machine ID
	// otherwise, the control plane will assign a new machine ID
	machineID := cliContext.String("machine-id") // can be empty

	gpuCount := cliContext.String("gpu-count")

	req, err := pkgmachineinfo.CreateLoginRequest(token, nvmlInstance, machineID, gpuCount)
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	publicIP := cliContext.String("public-ip")
	if publicIP != "" { // overwrite if not empty
		req.Network.PublicIP = publicIP
	}

	privateIP := cliContext.String("private-ip")
	if privateIP != "" { // overwrite if not empty
		req.Network.PrivateIP = privateIP
	}

	// machine ID has not been assigned yet
	// thus request one and blocks until the login request is processed
	endpoint := cliContext.String("endpoint")
	loginResp, err := login.SendRequest(rootCtx, endpoint, *req)
	if err != nil {
		return err
	}

	// persist only after the successful login
	if err := gpudstate.SetMetadata(rootCtx, dbRW, gpudstate.MetadataKeyEndpoint, endpoint); err != nil {
		return fmt.Errorf("failed to record endpoint: %w", err)
	}
	if err := gpudstate.SetMetadata(rootCtx, dbRW, gpudstate.MetadataKeyMachineID, loginResp.MachineID); err != nil {
		return fmt.Errorf("failed to record machine ID: %w", err)
	}
	if err := gpudstate.SetMetadata(rootCtx, dbRW, gpudstate.MetadataKeyToken, loginResp.Token); err != nil {
		return fmt.Errorf("failed to record session token: %w", err)
	}
	if err := gpudstate.SetMetadata(rootCtx, dbRW, gpudstate.MetadataKeyPublicIP, req.Network.PublicIP); err != nil {
		return fmt.Errorf("failed to record public IP: %w", err)
	}
	if err := gpudstate.SetMetadata(rootCtx, dbRW, gpudstate.MetadataKeyPrivateIP, req.Network.PrivateIP); err != nil {
		return fmt.Errorf("failed to record private IP: %w", err)
	}

	fifoFile, err := config.DefaultFifoFile()
	if err != nil {
		return fmt.Errorf("failed to get fifo file: %w", err)
	}

	// for GPUd >= v0.5, we assume "gpud login" first
	// and then "gpud up"
	// we still need this in case "gpud up" and then "gpud login" afterwards
	if err := server.WriteToken(token, fifoFile); err != nil {
		log.Logger.Debugw("failed to write token -- login before first gpud run/up", "error", err)
	}

	fmt.Printf("%s successfully logged in with machine id %s\n", cmdcommon.CheckMark, loginResp.MachineID)
	return nil
}
