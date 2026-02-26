package run

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/config"
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
