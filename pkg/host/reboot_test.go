package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRunReboot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test with a non-root user
	err := runReboot(ctx, "echo reboot")
	assert.NoError(t, err)
}

func TestLastRebootHelper(t *testing.T) {
	tests := []struct {
		name    string
		lines   [][]byte
		want    time.Time
		wantErr bool
	}{
		{
			name: "valid time format",
			lines: [][]byte{
				[]byte("2025-02-10 14:30:00"),
			},
			want: func() time.Time {
				// Calculate expected result the same way as LastRebootHelper
				t, _ := time.ParseInLocation("2006-01-02 15:04:05", "2025-02-10 14:30:00", time.Local)
				return t.UTC()
			}(),
			wantErr: false,
		},
		{
			name:    "empty input",
			lines:   [][]byte{},
			wantErr: true,
		},
		{
			name: "invalid time format",
			lines: [][]byte{
				[]byte("invalid-time"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LastRebootHelper(tt.lines)
			if (err != nil) != tt.wantErr {
				t.Errorf("LastRebootHelper() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("LastRebootHelper() = %v, want %v", got, tt.want)
			}
		})
	}
}
