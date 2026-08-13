package common

import (
	"context"
	"errors"
	"flag"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	componentsos "github.com/leptonai/gpud/components/os"
)

func newOSBlockedProcessCLIContext(t *testing.T, regexes *string, threshold *int) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("gpud-test", flag.ContinueOnError)
	set.String("os-blocked-process-name-regexes", "", "")
	set.Int("os-blocked-process-persistence-threshold", 0, "")

	if regexes != nil {
		require.NoError(t, set.Set("os-blocked-process-name-regexes", *regexes))
	}
	if threshold != nil {
		require.NoError(t, set.Set("os-blocked-process-persistence-threshold", strconv.Itoa(*threshold)))
	}

	app := cli.NewApp()
	return cli.NewContext(app, set, nil)
}

func withDefaultBlockedProcessThresholds(t *testing.T) {
	t.Helper()
	original := componentsos.GetDefaultBlockedProcessThresholds()
	originalStartup := componentsos.GetStartupBlockedProcessThresholds()
	t.Cleanup(func() {
		require.NoError(t, componentsos.SetDefaultBlockedProcessThresholds(original))
		componentsos.SetStartupBlockedProcessThresholds(originalStartup)
	})
}

func gpuDetector(hasGPU bool, err error) func(ctx context.Context) (bool, error) {
	return func(ctx context.Context) (bool, error) {
		return hasGPU, err
	}
}

func TestApplyOSBlockedProcessThresholds(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name          string
		regexes       *string
		threshold     *int
		hasGPU        bool
		detectErr     error
		oneOffScan    bool
		wantThreshold int
		wantRegexes   []string
		wantErr       bool
	}{
		{
			name:          "daemon on GPU machine auto-enables nvidia regexes with default threshold",
			hasGPU:        true,
			wantThreshold: componentsos.DefaultBlockedProcessPersistenceThreshold,
			wantRegexes:   []string{"^nvidia"},
		},
		{
			name:          "one-off scan on GPU machine auto-lowers threshold to 1",
			hasGPU:        true,
			oneOffScan:    true,
			wantThreshold: 1,
			wantRegexes:   []string{"^nvidia"},
		},
		{
			name:          "no GPU keeps the check disabled",
			hasGPU:        false,
			wantThreshold: componentsos.DefaultBlockedProcessPersistenceThreshold,
			wantRegexes:   []string{},
		},
		{
			name:          "detection error keeps the check disabled",
			detectErr:     errors.New("lspci not found"),
			wantThreshold: componentsos.DefaultBlockedProcessPersistenceThreshold,
			wantRegexes:   []string{},
		},
		{
			name:          "explicit regexes honored without GPU",
			regexes:       strPtr("^dd$,uvm_"),
			hasGPU:        false,
			wantThreshold: componentsos.DefaultBlockedProcessPersistenceThreshold,
			wantRegexes:   []string{"^dd$", "uvm_"},
		},
		{
			name:          "explicitly empty regexes disable the check even on GPU machines",
			regexes:       strPtr(""),
			hasGPU:        true,
			wantThreshold: componentsos.DefaultBlockedProcessPersistenceThreshold,
			wantRegexes:   []string{},
		},
		{
			name:          "explicit threshold honored, regexes auto-filled on GPU machine",
			threshold:     intPtr(3),
			hasGPU:        true,
			oneOffScan:    true,
			wantThreshold: 3,
			wantRegexes:   []string{"^nvidia"},
		},
		{
			name:          "explicit regexes and threshold skip GPU detection",
			regexes:       strPtr("^dd$"),
			threshold:     intPtr(7),
			hasGPU:        true,
			wantThreshold: 7,
			wantRegexes:   []string{"^dd$"},
		},
		{
			name:    "invalid regex fails",
			regexes: strPtr("(("),
			hasGPU:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withDefaultBlockedProcessThresholds(t)

			ctx := newOSBlockedProcessCLIContext(t, tt.regexes, tt.threshold)
			err := ApplyOSBlockedProcessThresholds(ctx, gpuDetector(tt.hasGPU, tt.detectErr), tt.oneOffScan)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			got := componentsos.GetDefaultBlockedProcessThresholds()
			assert.Equal(t, tt.wantThreshold, got.PersistenceThreshold)
			if len(tt.wantRegexes) == 0 {
				assert.Empty(t, got.NameRegexes)
			} else {
				assert.Equal(t, tt.wantRegexes, got.NameRegexes)
			}

			// the startup baseline (session updateConfig fallback target) must
			// be recorded with the same resolved thresholds
			startup := componentsos.GetStartupBlockedProcessThresholds()
			assert.Equal(t, got.PersistenceThreshold, startup.PersistenceThreshold)
			if len(tt.wantRegexes) == 0 {
				assert.Empty(t, startup.NameRegexes)
			} else {
				assert.Equal(t, tt.wantRegexes, startup.NameRegexes)
			}
		})
	}
}

func TestApplyOSBlockedProcessThresholds_NilDetector(t *testing.T) {
	withDefaultBlockedProcessThresholds(t)

	ctx := newOSBlockedProcessCLIContext(t, nil, nil)
	require.NoError(t, ApplyOSBlockedProcessThresholds(ctx, nil, false))

	got := componentsos.GetDefaultBlockedProcessThresholds()
	assert.Equal(t, componentsos.DefaultBlockedProcessPersistenceThreshold, got.PersistenceThreshold)
	assert.Empty(t, got.NameRegexes, "nil detector must leave the check disabled")
}
