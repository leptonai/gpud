package login

import (
	"context"
	"errors"
	"fmt"
	"time"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/server"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

var ErrEmptyToken = errors.New("token is empty")

// LoginConfig contains the configuration for the login operation.
type LoginConfig struct {
	Token     string
	Endpoint  string
	MachineID string // optional: can be empty
	NodeGroup string // optional

	DataDir string

	// GPUCount is the number of GPUs to be reported to the control plane.
	// If not specified, the control plane will use the detected number of GPUs.
	GPUCount string

	PublicIP  string // optional: overrides detected public IP
	PrivateIP string // optional: overrides detected private IP
}

// Login performs the login operation with the control plane.
// This function extracts the core login logic from the original login command.
//
// It handles the following scenarios based on the Login API Specification:
//
// Success:
// - HTTP 200 OK: Returns Machine ID and Session Token.
//
// Failures:
// - Validation Errors (HTTP 400):
//   - Invalid JSON: "cannot parse json, ..."
//   - Missing Machine Info: "machine info cannot be nil"
//   - Missing Token: "token is required"
//   - Missing ID/NodeGroup: "must specify a machine id or a node group id"
//
// - Token Validation Errors:
//   - Invalid Token (HTTP 401): "invalid token"
//   - Token Validation Failed (HTTP 500): "failed to validate token"
//
// - Machine/Node Group Validation Errors:
//   - Machine Not Found (HTTP 404): "machine not found"
//   - Node Group Mismatch (HTTP 400): "node group does not match"
//   - Node Group Not Found (HTTP 404): "node group <name> not found"
//   - Forbidden Access (HTTP 403): "only allowed to check in machines your owned"
//   - Forbidden Node Group (HTTP 403): "node group is not owned by the workspace"
//
// - Internal Server Errors (HTTP 500):
//   - Session Token Error: "failed to find session token"
//   - Machine Retrieval Error: "failed to get machine"
//   - Update Status Error: "failed to update machine info"
//   - Node Group Error: "failed to find node group"
//   - ID Generation Error: "failed to generate id"
//   - Machine Creation Error: "failed to add machine"
//   - Login Finalization Error: "failed to login, please try again"
func Login(ctx context.Context, cfg LoginConfig) error {
	if cfg.Token == "" {
		return ErrEmptyToken
	}

	dataDir, err := config.ResolveDataDir(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("failed to resolve data dir: %w", err)
	}

	log.Logger.Debugw("getting state file")
	stateFile := config.StateFilePath(dataDir)
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

	// in case the table has not been created
	log.Logger.Debugw("creating metadata table")
	if err := pkgmetadata.CreateTableMetadata(ctx, dbRW); err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}
	log.Logger.Debugw("successfully created metadata table")

	log.Logger.Debugw("creating session states table")
	if err := sessionstates.CreateTable(ctx, dbRW); err != nil {
		return fmt.Errorf("failed to create session states table: %w", err)
	}
	log.Logger.Debugw("successfully created session states table")

	log.Logger.Debugw("reading machine ID")
	prevMachineID, err := pkgmetadata.ReadMachineID(ctx, dbRO)
	if err != nil {
		return err
	}
	log.Logger.Debugw("successfully read machine ID")

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

	// previous/existing machine ID is not found (can be empty)
	// if specified, the control plane will validate the machine ID
	// otherwise, the control plane will assign a new machine ID
	loginCreatedAt := time.Now()
	log.Logger.Debugw("creating login request")
	req, err := pkgmachineinfo.CreateLoginRequest(cfg.Token, cfg.MachineID, cfg.NodeGroup, cfg.GPUCount, nvmlInstance)
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	log.Logger.Debugw("successfully created login request", "duration", time.Since(loginCreatedAt))

	if cfg.PublicIP != "" { // overwrite if not empty
		req.Network.PublicIP = cfg.PublicIP
	}

	if cfg.PrivateIP != "" { // overwrite if not empty
		req.Network.PrivateIP = cfg.PrivateIP
	}

	// machine ID has not been assigned yet
	// thus request one and blocks until the login request is processed
	loginSentAt := time.Now()
	log.Logger.Debugw("sending login request")
	loginResp, err := SendRequest(ctx, cfg.Endpoint, *req)
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

	// persist only after the successful login
	log.Logger.Debugw("recording endpoint")
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyEndpoint, cfg.Endpoint); err != nil {
		return fmt.Errorf("failed to record endpoint: %w", err)
	}
	log.Logger.Debugw("successfully recorded endpoint")

	log.Logger.Debugw("recording machine ID")
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, loginResp.MachineID); err != nil {
		return fmt.Errorf("failed to record machine ID: %w", err)
	}
	log.Logger.Debugw("successfully recorded machine ID")

	// here we persist the session token (not the user-specified machine registration token)
	// which has been generated by the DGXC Lepton control plane on its successful login
	// we persist here for future re-use in the session to the control plane
	log.Logger.Debugw("recording session token")
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, loginResp.Token); err != nil {
		return fmt.Errorf("failed to record session token: %w", err)
	}
	log.Logger.Debugw("successfully recorded session token")

	log.Logger.Debugw("recording public IP")
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyPublicIP, req.Network.PublicIP); err != nil {
		return fmt.Errorf("failed to record public IP: %w", err)
	}
	log.Logger.Debugw("successfully recorded public IP")

	log.Logger.Debugw("recording private IP")
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyPrivateIP, req.Network.PrivateIP); err != nil {
		return fmt.Errorf("failed to record private IP: %w", err)
	}
	log.Logger.Debugw("successfully recorded private IP")

	log.Logger.Debugw("getting fifo file")
	fifoFile := config.FifoFilePath(dataDir)
	log.Logger.Debugw("successfully got fifo file")

	log.Logger.Debugw("recording login success")
	if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyControlPlaneLoginSuccess, fmt.Sprintf("%d", time.Now().Unix())); err != nil {
		return fmt.Errorf("failed to record login success: %w", err)
	}
	log.Logger.Debugw("successfully recorded login success")

	// for GPUd >= v0.5, we assume "gpud login" first
	// and then "gpud up"
	// we still need this in case "gpud up" and then "gpud login" afterwards
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
				continue // only print invalid results
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

func serverRunning() bool {
	if systemd.SystemctlExists() {
		log.Logger.Debugw("checking if gpud.service is active")
		active, err := systemd.IsActive("gpud.service")
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
