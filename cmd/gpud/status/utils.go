package status

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func checkLoginSuccess(loginSuccess, machineID string, lastState *sessionstates.State) error {
	if loginSuccess == "" {
		return nil
	}

	ts, err := strconv.ParseInt(loginSuccess, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse login success: %w", err)
	}
	loginTimeUTC := time.Unix(ts, 0)
	nowUTC := time.Now().UTC()
	loginTimeHumanized := humanize.RelTime(loginTimeUTC, nowUTC, "ago", "from now")

	// If the most recent session activity is a failure that happened after the
	// original login success, the token is likely expired or invalid.
	// Show a warning instead of a green checkmark to avoid misleading operators.
	if lastState != nil && !lastState.Success && lastState.Timestamp > ts {
		fmt.Printf("%s login success at %s (machine id: %s) -- but session is currently failing, see login activity above\n", cmdcommon.WarningSign, loginTimeHumanized, machineID)
	} else {
		fmt.Printf("%s login success at %s (machine id: %s)\n", cmdcommon.CheckMark, loginTimeHumanized, machineID)
	}

	return nil
}

func displayLoginStatus(ctx context.Context, dbRO *sql.DB) (*sessionstates.State, error) {
	status, err := sessionstates.ReadLast(ctx, dbRO)
	if err != nil {
		// Handle the case where the session_states table doesn't exist yet.
		// This can happen if gpud status is run before gpud run/up has ever been executed,
		// or if the database was created with an older version that didn't have this table.
		if sqlite.IsNoSuchTableError(err) {
			fmt.Printf("No login activity recorded\n")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read login status: %w", err)
	}

	if status == nil {
		fmt.Printf("No login activity recorded\n")
		return nil, nil
	}

	statusTimeUTC := time.Unix(status.Timestamp, 0)
	nowUTC := time.Now().UTC()
	statusTimeHumanized := humanize.RelTime(statusTimeUTC, nowUTC, "ago", "from now")

	if status.Success {
		fmt.Printf("%s login activity: success at %s\n", cmdcommon.CheckMark, statusTimeHumanized)
	} else {
		fmt.Printf("%s login activity: failure at %s - %s\n", cmdcommon.WarningSign, statusTimeHumanized, status.Message)
	}

	// Check for any failures and warn if present
	hasFailures, err := sessionstates.HasAnyFailures(ctx, dbRO)
	if err != nil {
		return nil, fmt.Errorf("failed to check for login failures: %w", err)
	}

	if hasFailures {
		fmt.Printf("%s warning: there are login failure entries in the history\n", cmdcommon.WarningSign)
	}

	return status, nil
}
