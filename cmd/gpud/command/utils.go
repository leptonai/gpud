package command

import (
	"context"
	"fmt"

	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/config"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// GetUID returns the machine ID from the state file.
// Returns an empty string and sql.ErrNoRows if the machine ID is not found.
// Assumes that the state file is already opened and machine ID is already created.
func GetUID(ctx context.Context) (string, error) {
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return "", fmt.Errorf("failed to get state file: %w", err)
	}

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return "", fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()

	return state.GetMachineID(ctx, dbRO)
}
