package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
)

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

func getSessionCredentialsOptions(dbInMemory bool, dataDir string, controlPlaneEndpoint string) []config.OpOption {
	// When --db-in-memory is enabled, read session credentials from the persistent state file
	// and pass them to the config so the server can seed them into the in-memory database.
	// This works because login.Login() ALWAYS writes to persistent file (via dataDir),
	// and only the server respects --db-in-memory for its runtime database.
	//
	// IMPORTANT: The endpoint MUST also be seeded because the server reads the endpoint
	// from the metadata DB (not from config) for session keepalive.
	if !dbInMemory {
		return nil
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer readCancel()

	sessionToken, assignedMachineID, endpoint, readErr := readSessionCredentialsFromPersistentFile(readCtx, dataDir)

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
		return nil
	}

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
		return []config.OpOption{
			config.WithSessionToken(sessionToken),
			config.WithSessionMachineID(assignedMachineID),
			config.WithSessionEndpoint(endpoint),
		}
	}

	// Credentials were read but are incomplete - this may indicate a partial state
	log.Logger.Warnw(
		"db-in-memory mode enabled but session credentials are incomplete",
		"hasToken", sessionToken != "",
		"hasMachineID", assignedMachineID != "",
		"hasEndpoint", endpoint != "",
	)
	return nil
}
