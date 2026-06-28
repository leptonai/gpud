package login

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/server"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

// ErrEmptyToken reports a login attempt without a registration token.
var ErrEmptyToken = errors.New("token is empty")

// LoginConfig contains the configuration for the login operation.
//
//nolint:revive // Kept for CLI/package API stability.
type LoginConfig struct {
	Token     string
	Endpoint  string
	MachineID string // optional: can be empty
	NodeGroup string // optional

	// MachineIDOverwrite, when true, allows an explicitly-supplied MachineID that
	// differs from the locally-persisted one to replace it: the persisted login
	// identity (machine ID + session token, in the metadata table) is discarded
	// and the next login checks in using the supplied MachineID. Health/system
	// state (reboot history, events) in other tables is preserved.
	//
	// When false (the default), a differing MachineID is rejected with an error
	// instead of silently retargeting the node. This is the safe default for
	// host/systemd installs; the container/DaemonSet pattern sets it true so a
	// node re-enrolled with a new machine object checks in automatically.
	MachineIDOverwrite bool

	// RefreshSessionToken, when true, forces a fresh login on every start even
	// when a machine ID is already persisted -- instead of taking the "skip login"
	// fast path and reusing the persisted session token.
	//
	// The control plane looks up the current workspace-scoped session token on
	// every successful login, so re-login persists the token currently served by
	// the control plane. This handles workspace token rotation at the cost of one
	// extra login round-trip per start. The container/DaemonSet pattern sets it
	// true; the default (false) preserves the skip-login optimization for
	// host/systemd installs.
	RefreshSessionToken bool

	DataDir string

	// GPUCount is the number of GPUs to be reported to the control plane.
	// If not specified, the control plane will use the detected number of GPUs.
	GPUCount string

	// NodeLabels contains Kubernetes label key/value pairs to attach during login.
	// Keys without the managed "user.node.lepton.ai/" prefix are normalized before validation and sending.
	// Nil means "leave labels unchanged"; an empty but non-nil map means "clear labels".
	NodeLabels map[string]string

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
//
// reconcileMachineID reconciles an explicitly-supplied machine ID against the
// one already persisted in the local state DB, returning the machine ID that
// the rest of the login flow should treat as "previous".
//
// In the container/DaemonSet pattern, /var/lib/gpud is a node-scoped hostPath
// that outlives the machine object: when a node is deleted from its node group
// and rejoined, it receives a new requestedMachineID (for the new machine object,
// e.g. from the node label) while the OLD machine ID is still persisted on disk. Without
// this check, gpud would "skip login" on the stale ID and run under a mismatched
// identity (the persisted session token belongs to the old machine) -- so the
// node never joins.
//
// Behavior:
//   - No persisted ID, no supplied ID, or they already match -> no-op (returns
//     the persisted ID unchanged). This is the non-container path and is
//     completely unaffected.
//   - Supplied ID differs and overwrite=false -> fail loudly rather than
//     silently retarget the persisted identity.
//   - Supplied ID differs and overwrite=true  -> discard the persisted login
//     identity (metadata table only) and return "" so the caller checks in with
//     the requested machine ID. Reboot/event history lives in separate tables
//     and is intentionally NOT touched.
func reconcileMachineID(ctx context.Context, dbRW *sql.DB, prevMachineID, requestedMachineID string, overwrite bool) (string, error) {
	if prevMachineID == "" || requestedMachineID == "" || requestedMachineID == prevMachineID {
		return prevMachineID, nil
	}

	if !overwrite {
		return prevMachineID, fmt.Errorf(
			"persisted machine ID %q differs from requested machine ID %q; "+
				"pass --machine-id-overwrite to discard the persisted login identity and "+
				"check in with the requested machine (use this only if you intend to change the machine identity)",
			prevMachineID, requestedMachineID,
		)
	}

	// Loud, auditable: we are dropping a previously-registered identity.
	log.Logger.Warnw("!!! MACHINE ID OVERWRITE !!! discarding persisted login identity and checking in with requested machine ID",
		"persistedMachineID", prevMachineID,
		"requestedMachineID", requestedMachineID,
	)
	fmt.Printf("%s machine ID changed: overwriting %s -> %s (discarding persisted login identity)\n",
		cmdcommon.WarningSign, prevMachineID, requestedMachineID)

	if err := pkgmetadata.DeleteAllMetadata(ctx, dbRW); err != nil {
		return prevMachineID, fmt.Errorf("failed to clear persisted login identity for machine-id overwrite: %w", err)
	}
	return "", nil
}

func Login(ctx context.Context, cfg LoginConfig) error {
	if cfg.Token == "" {
		return ErrEmptyToken
	}

	normalizedNodeLabels, err := normalizeNodeLabels(cfg.NodeLabels)
	if err != nil {
		return fmt.Errorf("invalid node labels: %w", err)
	}
	cfg.NodeLabels = normalizedNodeLabels

	desiredNodeLabelsJSON, err := canonicalNodeLabels(cfg.NodeLabels)
	if err != nil {
		return fmt.Errorf("invalid node labels: %w", err)
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
	defer func() {
		_ = dbRW.Close()
	}()
	log.Logger.Debugw("successfully opened state file for writing")

	log.Logger.Debugw("opening state file for reading")
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer func() {
		_ = dbRO.Close()
	}()
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

	// Reconcile an explicitly-supplied machine ID against the persisted one
	// before deciding whether to skip login. See reconcileMachineID for the
	// container/DaemonSet rationale.
	prevMachineID, err = reconcileMachineID(ctx, dbRW, prevMachineID, cfg.MachineID, cfg.MachineIDOverwrite)
	if err != nil {
		return err
	}

	reloginExistingMachine := false
	if prevMachineID != "" {
		shouldRefreshLogin, err := shouldRefreshLoginForNodeLabels(ctx, dbRO, desiredNodeLabelsJSON, cfg.NodeLabels != nil)
		if err != nil {
			return fmt.Errorf("failed to read previous node labels: %w", err)
		}
		if shouldRefreshLogin || cfg.RefreshSessionToken {
			reloginExistingMachine = true
			// The control plane returns its current session token on every successful
			// login, so re-running login here re-fetches that token (and/or refreshes
			// node labels) instead of reusing a possibly-stale persisted one.
			log.Logger.Infow("re-running login for already-assigned machine",
				"machineID", prevMachineID,
				"nodeLabelsChanged", shouldRefreshLogin,
				"refreshSessionToken", cfg.RefreshSessionToken,
			)
		} else {
			fmt.Printf("machine ID %s already assigned (skipping login)\n", prevMachineID)
			return nil
		}
	}

	if reloginExistingMachine {
		// A relogin is a refresh for this exact machine, not an opportunity to point this
		// daemon at a different control-plane record. reconcileMachineID above has already
		// guaranteed the caller is not retargeting a different machine ID: a mismatch either
		// returned an error, or (with --machine-id-overwrite) cleared the persisted identity
		// and reset prevMachineID to "", which drops us out of this relogin path entirely.
		//
		// Use the persisted machine ID for the refresh request even when the caller omitted it
		// so the control plane sees an unambiguous relogin for the machine already assigned to
		// this host.
		cfg.MachineID = prevMachineID
	}

	if prevMachineID != "" && cfg.NodeGroup == "" {
		// Existing machines relogin by machine ID; node group is only needed for new-machine login.
		log.Logger.Debugw("using existing machine login path", "machineID", cfg.MachineID)
	}

	if prevMachineID != "" && cfg.NodeLabels != nil {
		log.Logger.Debugw("node labels configured for relogin", "machineID", cfg.MachineID, "nodeLabels", cfg.NodeLabels)
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
	req.NodeLabels = cfg.NodeLabels
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

	if cfg.NodeLabels != nil {
		log.Logger.Debugw("recording last sent node labels")
		if err := pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyLastSentNodeLabels, desiredNodeLabelsJSON); err != nil {
			return fmt.Errorf("failed to record last sent node labels: %w", err)
		}
		log.Logger.Debugw("successfully recorded last sent node labels")
	}

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

func shouldRefreshLoginForNodeLabels(ctx context.Context, dbRO *sql.DB, desiredNodeLabelsJSON string, nodeLabelsConfigured bool) (bool, error) {
	if !nodeLabelsConfigured {
		return false, nil
	}

	prevNodeLabelsJSON, err := pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyLastSentNodeLabels)
	if err != nil {
		return false, err
	}

	return prevNodeLabelsJSON != desiredNodeLabelsJSON, nil
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
