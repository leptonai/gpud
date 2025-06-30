package status

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
)

func TestCheckLoginSuccess(t *testing.T) {
	tests := []struct {
		name          string
		loginSuccess  string
		machineID     string
		expectedError error
		expectedOut   string
	}{
		{
			name:          "valid timestamp",
			loginSuccess:  strconv.FormatInt(time.Now().Add(-1*time.Hour).Unix(), 10),
			machineID:     "test-machine-123",
			expectedError: nil,
			expectedOut:   fmt.Sprintf("%s login success at", cmdcommon.CheckMark),
		},
		{
			name:          "empty login success",
			loginSuccess:  "",
			machineID:     "test-machine-456",
			expectedError: nil,
			expectedOut:   fmt.Sprintf("%s login information not found", cmdcommon.CheckMark),
		},
		{
			name:          "invalid timestamp",
			loginSuccess:  "invalid-timestamp",
			machineID:     "test-machine-789",
			expectedError: fmt.Errorf("failed to parse login success: strconv.ParseInt: parsing \"invalid-timestamp\": invalid syntax"),
			expectedOut:   "",
		},
		{
			name:          "future timestamp",
			loginSuccess:  strconv.FormatInt(time.Now().Add(1*time.Hour).Unix(), 10),
			machineID:     "test-machine-future",
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

			err := checkLoginSuccess(tt.loginSuccess, tt.machineID)

			// Restore stdout
			w.Close()
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
