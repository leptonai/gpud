package status

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestCheckLoginSuccess(t *testing.T) {
	tests := []struct {
		name          string
		loginSuccess  string
		machineID     string
		lastState     *sessionstates.State
		expectedError error
		expectedOut   string
	}{
		{
			name:          "valid timestamp no last state",
			loginSuccess:  strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10),
			machineID:     "test-machine-123",
			lastState:     nil,
			expectedError: nil,
			expectedOut:   fmt.Sprintf("%s login success at", cmdcommon.CheckMark),
		},
		{
			name:          "empty login success",
			loginSuccess:  "",
			machineID:     "test-machine-456",
			lastState:     nil,
			expectedError: nil,
			expectedOut:   "", // No output expected when loginSuccess is empty
		},
		{
			name:          "invalid timestamp",
			loginSuccess:  "invalid-timestamp",
			machineID:     "test-machine-789",
			lastState:     nil,
			expectedError: fmt.Errorf("failed to parse login success: strconv.ParseInt: parsing \"invalid-timestamp\": invalid syntax"),
			expectedOut:   "",
		},
		{
			name:          "future timestamp",
			loginSuccess:  strconv.FormatInt(time.Now().Add(1*time.Hour).Unix(), 10),
			machineID:     "test-machine-future",
			lastState:     nil,
			expectedError: nil,
			expectedOut:   fmt.Sprintf("%s login success at", cmdcommon.CheckMark),
		},
		{
			name:         "valid timestamp with last state success",
			loginSuccess: strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10),
			machineID:    "test-machine-ok",
			lastState: &sessionstates.State{
				Timestamp: time.Now().Unix(),
				Success:   true,
				Message:   "Session connected successfully",
			},
			expectedError: nil,
			expectedOut:   fmt.Sprintf("%s login success at", cmdcommon.CheckMark),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := checkLoginSuccess(tt.loginSuccess, tt.machineID, tt.lastState)

			// Restore stdout
			require.NoError(t, w.Close())
			os.Stdout = old

			// Read captured output
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			// Check error
			if tt.expectedError != nil {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError.Error())
			} else {
				require.NoError(t, err)
			}

			// Check output
			if tt.expectedOut != "" {
				assert.Contains(t, output, tt.expectedOut)
				if tt.loginSuccess != "" {
					// For valid timestamps, check that machine ID is in output
					assert.Contains(t, output, tt.machineID)
				}
			}

			// Additional checks for valid timestamps
			if tt.loginSuccess != "" && err == nil {
				// Should contain "ago" or "from now"
				assert.True(t, strings.Contains(output, "ago") || strings.Contains(output, "from now"))
			}
		})
	}
}

// TestCheckLoginSuccessStaleDetection verifies that checkLoginSuccess shows a
// warning when the most recent session activity is a failure newer than the
// original login success timestamp. This is the user-facing part of the LEP-4748 fix.
func TestCheckLoginSuccessStaleDetection(t *testing.T) {
	loginTS := time.Now().Add(-24 * time.Hour).Unix() // Login was 24h ago

	tests := []struct {
		name        string
		lastState   *sessionstates.State
		wantWarning bool
	}{
		{
			name: "failure newer than login shows warning",
			lastState: &sessionstates.State{
				Timestamp: time.Now().Unix(), // Failure is NOW, much newer than login
				Success:   false,
				Message:   "HTTP 403: forbidden",
			},
			wantWarning: true,
		},
		{
			name: "failure older than login shows checkmark",
			lastState: &sessionstates.State{
				Timestamp: loginTS - 3600, // Failure BEFORE login
				Success:   false,
				Message:   "HTTP 403: forbidden",
			},
			wantWarning: false,
		},
		{
			name: "success newer than login shows checkmark",
			lastState: &sessionstates.State{
				Timestamp: time.Now().Unix(),
				Success:   true,
				Message:   "ok",
			},
			wantWarning: false,
		},
		{
			name:        "nil last state shows checkmark",
			lastState:   nil,
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			loginSuccess := strconv.FormatInt(loginTS, 10)
			err := checkLoginSuccess(loginSuccess, "machine-stale-test", tt.lastState)

			require.NoError(t, w.Close())
			os.Stdout = old

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			require.NoError(t, err)

			if tt.wantWarning {
				assert.Contains(t, output, cmdcommon.WarningSign, "Expected warning sign for stale login")
				assert.Contains(t, output, "session is currently failing, see login activity above")
			} else {
				assert.Contains(t, output, cmdcommon.CheckMark, "Expected checkmark for valid login")
				assert.NotContains(t, output, "session is currently failing, see login activity above")
			}
		})
	}
}

func TestDisplayLoginStatus(t *testing.T) {
	t.Run("handles missing session_states table gracefully", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp(os.TempDir(), "status_test")
		require.NoError(t, err)
		defer func() {
			_ = os.RemoveAll(tmpDir)
		}()

		// Create a database without the session_states table
		dbFile := filepath.Join(tmpDir, "test.db")
		dbRW, err := sqlite.Open(dbFile)
		require.NoError(t, err)

		// Create some other table to ensure the database exists
		_, err = dbRW.Exec("CREATE TABLE other_table (id INTEGER PRIMARY KEY)")
		require.NoError(t, err)
		_ = dbRW.Close()

		// Open in read-only mode (like the status command does)
		dbRO, err := sqlite.Open(dbFile, sqlite.WithReadOnly(true))
		require.NoError(t, err)
		defer func() {
			_ = dbRO.Close()
		}()

		// Capture stdout
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Call displayLoginStatus - should not return an error
		ctx := context.Background()
		state, err := displayLoginStatus(ctx, dbRO)

		// Restore stdout
		require.NoError(t, w.Close())
		os.Stdout = old

		// Read captured output
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()

		// Verify no error, nil state, and appropriate output
		require.NoError(t, err, "displayLoginStatus should not return an error for missing table")
		assert.Nil(t, state, "Should return nil state when no table exists")
		assert.Contains(t, output, "No login activity recorded")
	})

	t.Run("returns last failure state", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbFile := filepath.Join(tmpDir, "test.db")

		dbRW, err := sqlite.Open(dbFile)
		require.NoError(t, err)

		require.NoError(t, sessionstates.CreateTable(context.Background(), dbRW))

		// Insert a failure entry
		failTS := time.Now().Unix()
		require.NoError(t, sessionstates.Insert(context.Background(), dbRW, failTS, false, "HTTP 403: forbidden"))
		_ = dbRW.Close()

		dbRO, err := sqlite.Open(dbFile, sqlite.WithReadOnly(true))
		require.NoError(t, err)
		defer func() { _ = dbRO.Close() }()

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		state, err := displayLoginStatus(context.Background(), dbRO)

		require.NoError(t, w.Close())
		os.Stdout = old

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()

		require.NoError(t, err)
		require.NotNil(t, state)
		assert.False(t, state.Success)
		assert.Equal(t, failTS, state.Timestamp)
		assert.Contains(t, output, "login activity: failure")
		assert.Contains(t, output, "HTTP 403")
		assert.Contains(t, output, "warning: there are login failure entries")
	})

	t.Run("returns last success state", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbFile := filepath.Join(tmpDir, "test.db")

		dbRW, err := sqlite.Open(dbFile)
		require.NoError(t, err)

		require.NoError(t, sessionstates.CreateTable(context.Background(), dbRW))

		successTS := time.Now().Unix()
		require.NoError(t, sessionstates.Insert(context.Background(), dbRW, successTS, true, "ok"))
		_ = dbRW.Close()

		dbRO, err := sqlite.Open(dbFile, sqlite.WithReadOnly(true))
		require.NoError(t, err)
		defer func() { _ = dbRO.Close() }()

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		state, err := displayLoginStatus(context.Background(), dbRO)

		require.NoError(t, w.Close())
		os.Stdout = old

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()

		require.NoError(t, err)
		require.NotNil(t, state)
		assert.True(t, state.Success)
		assert.Contains(t, output, "login activity: success")
		assert.NotContains(t, output, "warning")
	})
}
