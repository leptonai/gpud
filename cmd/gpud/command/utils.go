package command

import (
	"context"
	"fmt"

	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/config"
)

func GetUID(ctx context.Context) (string, error) {
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return "", fmt.Errorf("failed to get state file: %w", err)
	}
	db, err := state.Open(stateFile)
	if err != nil {
		return "", fmt.Errorf("failed to open state file: %w", err)
	}
	defer db.Close()
	uid, _, err := state.CreateMachineIDIfNotExist(ctx, db, "")
	if err != nil {
		return "", fmt.Errorf("failed to get machine uid: %w", err)
	}
	return uid, nil
}
