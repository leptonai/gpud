package command

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/config"
	gpudstate "github.com/leptonai/gpud/pkg/gpud-state"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/login"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/server"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func cmdLogin(cliContext *cli.Context) error {
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
	if err := gpudstate.CreateTableMachineMetadata(rootCtx, dbRW); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// if the login is the "first" operations before creating the db,
	// we need write mode
	// read-only mode fails with "no such file or directory"
	// if the db file does not already exist
	machineID, err := gpudstate.ReadMachineID(rootCtx, dbRW)
	if err != nil {
		return err
	}
	if machineID != "" {
		fmt.Printf("machine ID %s already assigned (skipping login)\n", machineID)
		return nil
	}

	endpoint := cliContext.String("endpoint")

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return fmt.Errorf("failed to create nvml instance: %w", err)
	}
	defer func() {
		if err := nvmlInstance.Shutdown(); err != nil {
			log.Logger.Debugw("failed to shutdown nvml instance", "error", err)
		}
	}()

	req, err := pkgmachineinfo.CreateLoginRequest(token, nvmlInstance, cliContext.String("machine-id"), cliContext.String("gpu-count"), cliContext.String("private-ip"), cliContext.String("public-ip"))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	// machine ID has not been assigned yet
	// thus request one and blocks until the login request is processed
	loginResp, err := login.SendRequest(rootCtx, endpoint, *req)
	if err != nil {
		return err
	}
	machineID = loginResp.MachineID
	sessionToken := loginResp.Token

	// consume the login response to persist the machine ID
	if err := gpudstate.RecordMachineID(rootCtx, dbRW, dbRO, machineID); err != nil {
		return fmt.Errorf("failed to record machine ID: %w", err)
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

	if err = gpudstate.UpdateLoginInfo(rootCtx, dbRW, machineID, sessionToken); err != nil {
		fmt.Println("machine logged in but failed to update token:", err)
	}

	fmt.Printf("%s successfully logged in with machine id %s\n", checkMark, loginResp.MachineID)
	return nil
}
