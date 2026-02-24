package scan

import (
	"context"
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	componentssxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	pkgscan "github.com/leptonai/gpud/pkg/scan"
)

// =============================================================================
// cmdScan Tests
// =============================================================================

// TestCmdScan_InvalidLogLevel tests cmdScan with an invalid log level.
func TestCmdScan_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("scan command invalid log level", t, func() {
		err := cmdScan(
			"invalid-level",
			0,     // gpuCount
			"",    // infinibandExpectedPortStates
			"",    // nvlinkExpectedLinkStates
			"",    // nfsCheckerConfigs
			"",    // ibClassRootDir
			"",    // gpuUUIDsWithRowRemappingPendingRaw
			"",    // gpuUUIDsWithRowRemappingFailedRaw
			"",    // gpuUUIDsWithHWSlowdownRaw
			"",    // gpuUUIDsWithHWSlowdownThermalRaw
			"",    // gpuUUIDsWithHWSlowdownPowerBrakeRaw
			"",    // gpuUUIDsWithGPULostRaw
			"",    // gpuUUIDsWithGPURequiresResetRaw
			"",    // gpuUUIDsWithFabricStateHealthSummaryUnhealthyRaw
			"",    // gpuProductNameOverride
			false, // containerdSocketMissing
			0,     // xidRebootThreshold
			false, // xidRebootThresholdIsSet
			0,     // temperatureMarginThresholdCelsius
			false, // temperatureMarginThresholdIsSet
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unrecognized level")
	})
}

// TestCmdScan_InvalidInfinibandJSON tests cmdScan with invalid infiniband JSON.
func TestCmdScan_InvalidInfinibandJSON(t *testing.T) {
	mockey.PatchConvey("scan command invalid infiniband JSON", t, func() {
		err := cmdScan(
			"info",
			0,
			"not-valid-json", // infinibandExpectedPortStates
			"", "", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.Error(t, err)
	})
}

// TestCmdScan_InvalidNvlinkJSON tests cmdScan with invalid nvlink JSON.
func TestCmdScan_InvalidNvlinkJSON(t *testing.T) {
	mockey.PatchConvey("scan command invalid nvlink JSON", t, func() {
		err := cmdScan(
			"info",
			0,
			"",               // infinibandExpectedPortStates
			"not-valid-json", // nvlinkExpectedLinkStates
			"", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.Error(t, err)
	})
}

// TestCmdScan_InvalidNFSJSON tests cmdScan with invalid NFS checker config JSON.
func TestCmdScan_InvalidNFSJSON(t *testing.T) {
	mockey.PatchConvey("scan command invalid NFS JSON", t, func() {
		err := cmdScan(
			"info",
			0,
			"",               // infinibandExpectedPortStates
			"",               // nvlinkExpectedLinkStates
			"not-valid-json", // nfsCheckerConfigs
			"", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.Error(t, err)
	})
}

// TestCmdScan_ScanError tests cmdScan when scan.Scan returns an error.
func TestCmdScan_ScanError(t *testing.T) {
	mockey.PatchConvey("scan command scan error", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return errors.New("scan failed: NVML not available")
		}).Build()

		err := cmdScan(
			"info",
			0, "", "", "", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "scan failed")
	})
}

// TestCmdScan_Success tests cmdScan with minimal valid parameters.
func TestCmdScan_Success(t *testing.T) {
	mockey.PatchConvey("scan command success", t, func() {
		scanCalled := false
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			scanCalled = true
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0, "", "", "", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.NoError(t, err)
		assert.True(t, scanCalled, "expected scan.Scan to be called")
	})
}

// TestCmdScan_WithGPUCount tests cmdScan with a GPU count set.
func TestCmdScan_WithGPUCount(t *testing.T) {
	mockey.PatchConvey("scan command with GPU count", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			8, // gpuCount
			"", "", "", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithXIDRebootThresholdPositive tests cmdScan with a positive xid reboot threshold.
func TestCmdScan_WithXIDRebootThresholdPositive(t *testing.T) {
	mockey.PatchConvey("scan command with xid reboot threshold positive", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0, "", "", "", "", "", "", "", "", "", "", "", "", "",
			false, // containerdSocketMissing
			5,     // xidRebootThreshold
			true,  // xidRebootThresholdIsSet
			0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithXIDRebootThresholdNonPositive tests cmdScan with a non-positive xid reboot threshold.
func TestCmdScan_WithXIDRebootThresholdNonPositive(t *testing.T) {
	mockey.PatchConvey("scan command with xid reboot threshold non-positive", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0, "", "", "", "", "", "", "", "", "", "", "", "", "",
			false, // containerdSocketMissing
			0,     // xidRebootThreshold (non-positive, should be ignored with warning)
			true,  // xidRebootThresholdIsSet
			0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithTemperatureMarginThreshold tests cmdScan with temperature margin threshold set.
func TestCmdScan_WithTemperatureMarginThreshold(t *testing.T) {
	mockey.PatchConvey("scan command with temperature margin threshold", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0, "", "", "", "", "", "", "", "", "", "", "", "", "", false, 0, false,
			10,   // temperatureMarginThresholdCelsius
			true, // temperatureMarginThresholdIsSet
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithDebugLevel tests cmdScan with debug log level (enables debug option).
func TestCmdScan_WithDebugLevel(t *testing.T) {
	mockey.PatchConvey("scan command with debug log level", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"debug",
			0, "", "", "", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_ValidLogLevels tests cmdScan with all valid log levels.
func TestCmdScan_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			mockey.PatchConvey("valid log level "+level, t, func() {
				mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
					return nil
				}).Build()

				err := cmdScan(
					level,
					0, "", "", "", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
				)
				require.NoError(t, err)
			})
		})
	}
}

// TestCmdScan_WithGPUUUIDs tests cmdScan with various GPU UUID parameters.
func TestCmdScan_WithGPUUUIDs(t *testing.T) {
	mockey.PatchConvey("scan command with GPU UUIDs", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0,                       // gpuCount
			"",                      // infinibandExpectedPortStates
			"",                      // nvlinkExpectedLinkStates
			"",                      // nfsCheckerConfigs
			"",                      // ibClassRootDir
			"GPU-uuid-1,GPU-uuid-2", // gpuUUIDsWithRowRemappingPendingRaw
			"GPU-uuid-3",            // gpuUUIDsWithRowRemappingFailedRaw
			"GPU-uuid-4",            // gpuUUIDsWithHWSlowdownRaw
			"",                      // gpuUUIDsWithHWSlowdownThermalRaw
			"",                      // gpuUUIDsWithHWSlowdownPowerBrakeRaw
			"",                      // gpuUUIDsWithGPULostRaw
			"",                      // gpuUUIDsWithGPURequiresResetRaw
			"",                      // gpuUUIDsWithFabricStateHealthSummaryUnhealthyRaw
			"H100-SXM",              // gpuProductNameOverride
			false,                   // containerdSocketMissing
			0, false, 0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithValidInfinibandJSON tests cmdScan with valid infiniband JSON config.
func TestCmdScan_WithValidInfinibandJSON(t *testing.T) {
	mockey.PatchConvey("scan command with valid infiniband JSON", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0,
			`{}`, // valid infiniband JSON (empty object)
			"", "", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithValidNvlinkJSON tests cmdScan with valid nvlink JSON config.
func TestCmdScan_WithValidNvlinkJSON(t *testing.T) {
	mockey.PatchConvey("scan command with valid nvlink JSON", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0,
			"",   // infinibandExpectedPortStates
			`{}`, // valid nvlink JSON (empty object)
			"", "", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithValidNFSJSON tests cmdScan with valid NFS checker config JSON.
func TestCmdScan_WithValidNFSJSON(t *testing.T) {
	mockey.PatchConvey("scan command with valid NFS JSON", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0,
			"",   // infinibandExpectedPortStates
			"",   // nvlinkExpectedLinkStates
			`[]`, // valid NFS JSON (empty array)
			"", "", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithInfinibandClassRootDir tests cmdScan with a custom infiniband class root dir.
func TestCmdScan_WithInfinibandClassRootDir(t *testing.T) {
	mockey.PatchConvey("scan command with infiniband class root dir", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"info",
			0,
			"",
			"",
			"",
			"/custom/infiniband/class", // ibClassRootDir
			"", "", "", "", "", "", "", "", "", false, 0, false, 0, false,
		)
		require.NoError(t, err)
	})
}

// TestCmdScan_WithAllConfigOptions tests cmdScan with all configuration options set.
func TestCmdScan_WithAllConfigOptions(t *testing.T) {
	mockey.PatchConvey("scan command with all config options", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := cmdScan(
			"debug",
			4,                       // gpuCount
			`{}`,                    // infinibandExpectedPortStates
			`{}`,                    // nvlinkExpectedLinkStates
			`[]`,                    // nfsCheckerConfigs
			"/sys/class/infiniband", // ibClassRootDir
			"GPU-uuid-1",            // gpuUUIDsWithRowRemappingPendingRaw
			"GPU-uuid-2",            // gpuUUIDsWithRowRemappingFailedRaw
			"GPU-uuid-3",            // gpuUUIDsWithHWSlowdownRaw
			"GPU-uuid-4",            // gpuUUIDsWithHWSlowdownThermalRaw
			"GPU-uuid-5",            // gpuUUIDsWithHWSlowdownPowerBrakeRaw
			"GPU-uuid-6",            // gpuUUIDsWithGPULostRaw
			"GPU-uuid-7",            // gpuUUIDsWithGPURequiresResetRaw
			"GPU-uuid-8",            // gpuUUIDsWithFabricStateHealthSummaryUnhealthyRaw
			"H100-SXM",              // gpuProductNameOverride
			false,                   // containerdSocketMissing
			5,                       // xidRebootThreshold
			true,                    // xidRebootThresholdIsSet
			10,                      // temperatureMarginThresholdCelsius
			true,                    // temperatureMarginThresholdIsSet
		)
		require.NoError(t, err)
	})
}

// =============================================================================
// CreateCommand Tests
// =============================================================================

// TestCreateCommand_ReturnsFunction tests that CreateCommand returns a non-nil function.
func TestCreateCommand_ReturnsFunction(t *testing.T) {
	fn := CreateCommand()
	assert.NotNil(t, fn)
}

func newTestCLIContext(t *testing.T, xidLookbackPeriod, sxidLookbackPeriod, eventsRetentionPeriod time.Duration) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("gpud-scan-test", flag.ContinueOnError)
	set.String("log-level", "info", "")
	set.Duration("xid-lookback-period", 0, "")
	set.Duration("sxid-lookback-period", 0, "")
	set.Duration("events-retention-period", 0, "")

	if xidLookbackPeriod > 0 {
		require.NoError(t, set.Set("xid-lookback-period", xidLookbackPeriod.String()))
	}
	if sxidLookbackPeriod > 0 {
		require.NoError(t, set.Set("sxid-lookback-period", sxidLookbackPeriod.String()))
	}
	if eventsRetentionPeriod > 0 {
		require.NoError(t, set.Set("events-retention-period", eventsRetentionPeriod.String()))
	}

	app := cli.NewApp()
	return cli.NewContext(app, set, nil)
}

func TestCreateCommand_SetLookbackPeriods(t *testing.T) {
	originalXidLookback := componentsxid.GetLookbackPeriod()
	originalSxidLookback := componentssxid.GetLookbackPeriod()
	t.Cleanup(func() {
		componentsxid.SetLookbackPeriod(originalXidLookback)
		componentssxid.SetLookbackPeriod(originalSxidLookback)
	})

	newXidLookback := 4 * time.Hour
	newSxidLookback := 9 * time.Hour
	ctx := newTestCLIContext(t, newXidLookback, newSxidLookback, 0)

	mockey.PatchConvey("create command sets xid/sxid lookback periods", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := CreateCommand()(ctx)
		require.NoError(t, err)
		assert.Equal(t, newXidLookback, componentsxid.GetLookbackPeriod())
		assert.Equal(t, newSxidLookback, componentssxid.GetLookbackPeriod())
	})
}

func TestCreateCommand_UsesEventsRetentionForLookback(t *testing.T) {
	originalXidLookback := componentsxid.GetLookbackPeriod()
	originalSxidLookback := componentssxid.GetLookbackPeriod()
	t.Cleanup(func() {
		componentsxid.SetLookbackPeriod(originalXidLookback)
		componentssxid.SetLookbackPeriod(originalSxidLookback)
	})

	eventsRetentionPeriod := 36 * time.Hour
	ctx := newTestCLIContext(t, 0, 0, eventsRetentionPeriod)

	mockey.PatchConvey("create command uses events retention period for xid/sxid lookback", t, func() {
		mockey.Mock(pkgscan.Scan).To(func(ctx context.Context, opts ...pkgscan.OpOption) error {
			return nil
		}).Build()

		err := CreateCommand()(ctx)
		require.NoError(t, err)
		assert.Equal(t, eventsRetentionPeriod, componentsxid.GetLookbackPeriod())
		assert.Equal(t, eventsRetentionPeriod, componentssxid.GetLookbackPeriod())
	})
}
