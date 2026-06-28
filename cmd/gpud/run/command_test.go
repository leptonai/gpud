package run

import (
	"context"
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/config"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgsqlite "github.com/leptonai/gpud/pkg/sqlite"
)

func TestParseInfinibandExcludeDevices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "commas and spaces only",
			input: " , , ",
			want:  nil,
		},
		{
			name:  "single device",
			input: "mlx5_0",
			want:  []string{"mlx5_0"},
		},
		{
			name:  "multiple devices with spaces and empties",
			input: " mlx5_0, ,mlx5_1 ,",
			want:  []string{"mlx5_0", "mlx5_1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseInfinibandExcludeDevices(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseRetentionPeriods(t *testing.T) {
	t.Parallel()

	newCLIContext := func(t *testing.T, metrics, deprecated, events time.Duration) *cli.Context {
		t.Helper()

		set := flag.NewFlagSet("gpud-test", flag.ContinueOnError)
		set.Duration("metrics-retention-period", 0, "")
		set.Duration("retention-period", 0, "")
		set.Duration("events-retention-period", 0, "")
		require.NoError(t, set.Set("metrics-retention-period", metrics.String()))
		require.NoError(t, set.Set("retention-period", deprecated.String()))
		require.NoError(t, set.Set("events-retention-period", events.String()))

		return cli.NewContext(cli.NewApp(), set, nil)
	}

	tests := []struct {
		name          string
		metrics       time.Duration
		deprecated    time.Duration
		events        time.Duration
		expectMetrics time.Duration
		expectEvents  time.Duration
	}{
		{
			name:          "metrics flag has highest precedence",
			metrics:       5 * time.Minute,
			deprecated:    7 * time.Minute,
			events:        24 * time.Hour,
			expectMetrics: 5 * time.Minute,
			expectEvents:  24 * time.Hour,
		},
		{
			name:          "deprecated flag is used when metrics flag is zero",
			metrics:       0,
			deprecated:    7 * time.Minute,
			events:        24 * time.Hour,
			expectMetrics: 7 * time.Minute,
			expectEvents:  24 * time.Hour,
		},
		{
			name:          "deprecated flag is used when metrics flag is negative",
			metrics:       -1 * time.Minute,
			deprecated:    7 * time.Minute,
			events:        24 * time.Hour,
			expectMetrics: 7 * time.Minute,
			expectEvents:  24 * time.Hour,
		},
		{
			name:          "defaults are used when all values are non-positive",
			metrics:       -1 * time.Minute,
			deprecated:    0,
			events:        -1 * time.Hour,
			expectMetrics: config.DefaultMetricsRetentionPeriod.Duration,
			expectEvents:  config.DefaultEventsRetentionPeriod.Duration,
		},
		{
			name:          "events default applies independently",
			metrics:       5 * time.Minute,
			deprecated:    0,
			events:        0,
			expectMetrics: 5 * time.Minute,
			expectEvents:  config.DefaultEventsRetentionPeriod.Duration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newCLIContext(t, tt.metrics, tt.deprecated, tt.events)
			metrics, events := parseRetentionPeriods(ctx)
			assert.Equal(t, tt.expectMetrics, metrics)
			assert.Equal(t, tt.expectEvents, events)
		})
	}
}

func TestPersistMetadataOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                       string
		persistedMachineID         string
		persistedToken             string
		controlPlaneEndpoint       string
		requestedMachineID         string
		machineIDOverwrite         bool
		controlPlaneLoginSucceeded bool
		wantMachineID              string
		wantToken                  string
		wantEndpoint               string
		errContains                string
	}{
		{
			name:                 "endpoint override without machine id is persisted",
			controlPlaneEndpoint: "gpud-manager.example.com",
			wantEndpoint:         "gpud-manager.example.com",
		},
		{
			name:               "initial machine id without login is allowed",
			requestedMachineID: "new-machine-id",
			wantMachineID:      "new-machine-id",
		},
		{
			name:               "matching machine id without login is allowed",
			persistedMachineID: "machine-id",
			persistedToken:     "session-token",
			requestedMachineID: "machine-id",
			wantMachineID:      "machine-id",
			wantToken:          "session-token",
		},
		{
			name:               "mismatch without overwrite is rejected",
			persistedMachineID: "old-machine-id",
			persistedToken:     "old-session-token",
			requestedMachineID: "new-machine-id",
			wantMachineID:      "old-machine-id",
			wantToken:          "old-session-token",
			errContains:        "pass --machine-id-overwrite",
		},
		{
			name:               "mismatch without a successful login is rejected",
			persistedMachineID: "old-machine-id",
			persistedToken:     "old-session-token",
			requestedMachineID: "new-machine-id",
			machineIDOverwrite: true,
			wantMachineID:      "old-machine-id",
			wantToken:          "old-session-token",
			errContains:        "pass --token",
		},
		{
			name:                       "successful login keeps control plane machine id",
			persistedMachineID:         "manager-machine-id",
			persistedToken:             "fresh-session-token",
			requestedMachineID:         "requested-machine-id",
			machineIDOverwrite:         true,
			controlPlaneLoginSucceeded: true,
			wantMachineID:              "manager-machine-id",
			wantToken:                  "fresh-session-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			stateFile := config.StateFilePath(t.TempDir())
			dbRW, err := pkgsqlite.Open(stateFile)
			require.NoError(t, err)
			require.NoError(t, pkgmetadata.CreateTableMetadata(ctx, dbRW))
			if tt.persistedMachineID != "" {
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyMachineID, tt.persistedMachineID))
			}
			if tt.persistedToken != "" {
				require.NoError(t, pkgmetadata.SetMetadata(ctx, dbRW, pkgmetadata.MetadataKeyToken, tt.persistedToken))
			}
			require.NoError(t, dbRW.Close())

			err = persistMetadataOverrides(ctx, stateFile, tt.controlPlaneEndpoint, tt.requestedMachineID, tt.machineIDOverwrite, tt.controlPlaneLoginSucceeded)
			if tt.errContains == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			}

			dbRO, err := pkgsqlite.Open(stateFile, pkgsqlite.WithReadOnly(true))
			require.NoError(t, err)
			defer func() { _ = dbRO.Close() }()

			machineID, err := pkgmetadata.ReadMachineID(ctx, dbRO)
			require.NoError(t, err)
			token, err := pkgmetadata.ReadToken(ctx, dbRO)
			require.NoError(t, err)
			endpoint, err := pkgmetadata.ReadMetadata(ctx, dbRO, pkgmetadata.MetadataKeyEndpoint)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMachineID, machineID)
			assert.Equal(t, tt.wantToken, token)
			assert.Equal(t, tt.wantEndpoint, endpoint)
		})
	}
}
