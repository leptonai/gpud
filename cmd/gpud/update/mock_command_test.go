package update

import (
	"errors"
	"flag"
	"io"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	pkgupdate "github.com/leptonai/gpud/pkg/update"
	"github.com/leptonai/gpud/version"
)

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-update-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("log-file", "", "")
	_ = flags.String("next-version", "", "")
	_ = flags.String("url", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// =============================================================================
// Command Tests
// =============================================================================

// TestCommand_InvalidLogLevel tests the command with an invalid log level.
func TestCommand_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("update command invalid log level", t, func() {
		cliContext := newCLIContext(t, []string{"--log-level", "invalid-level"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unrecognized level")
	})
}

// TestCommand_DetectLatestVersionError tests when auto-detecting the latest version fails.
func TestCommand_DetectLatestVersionError(t *testing.T) {
	mockey.PatchConvey("update command detect latest version error", t, func() {
		mockey.Mock(version.DetectLatestVersion).To(func() (string, error) {
			return "", errors.New("network error: cannot reach server")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "network error")
	})
}

// TestCommand_UpdateExecutableError tests when UpdateExecutable fails.
func TestCommand_UpdateExecutableError(t *testing.T) {
	mockey.PatchConvey("update command update executable error", t, func() {
		mockey.Mock(pkgupdate.UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			return errors.New("failed to download update")
		}).Build()

		cliContext := newCLIContext(t, []string{"--next-version", "1.2.3"})
		err := Command(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to download update")
	})
}

// TestCommand_SuccessWithExplicitVersion tests successful update with an explicit version and URL.
func TestCommand_SuccessWithExplicitVersion(t *testing.T) {
	mockey.PatchConvey("update command success with explicit version", t, func() {
		var capturedVersion, capturedURL string
		var capturedRequireRoot bool

		mockey.Mock(pkgupdate.UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			capturedVersion = targetVersion
			capturedURL = url
			capturedRequireRoot = requireRoot
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{"--next-version", "1.2.3", "--url", "https://custom.example.com/"})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, "1.2.3", capturedVersion)
		assert.Equal(t, "https://custom.example.com/", capturedURL)
		assert.True(t, capturedRequireRoot)
	})
}

// TestCommand_SuccessWithAutoDetectedVersion tests successful update with auto-detected version.
func TestCommand_SuccessWithAutoDetectedVersion(t *testing.T) {
	mockey.PatchConvey("update command success with auto-detected version", t, func() {
		mockey.Mock(version.DetectLatestVersion).To(func() (string, error) {
			return "2.0.0", nil
		}).Build()

		var capturedVersion string
		mockey.Mock(pkgupdate.UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			capturedVersion = targetVersion
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, "2.0.0", capturedVersion)
	})
}

// TestCommand_DefaultURL tests that the default URL prefix is used when no URL is provided.
func TestCommand_DefaultURL(t *testing.T) {
	mockey.PatchConvey("update command default URL", t, func() {
		var capturedURL string
		mockey.Mock(pkgupdate.UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			capturedURL = url
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{"--next-version", "1.2.3"})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, version.DefaultURLPrefix, capturedURL)
	})
}

// TestCommand_ValidLogLevels tests the command with various valid log levels.
func TestCommand_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			mockey.PatchConvey("valid log level "+level, t, func() {
				mockey.Mock(version.DetectLatestVersion).To(func() (string, error) {
					return "", errors.New("early exit")
				}).Build()

				cliContext := newCLIContext(t, []string{"--log-level", level})
				err := Command(cliContext)
				// Should fail at DetectLatestVersion, not at log level parsing
				require.Error(t, err)
				assert.Contains(t, err.Error(), "early exit")
			})
		})
	}
}

// TestCommand_WithLogFile tests the command with a log file flag.
func TestCommand_WithLogFile(t *testing.T) {
	mockey.PatchConvey("update command with log file", t, func() {
		mockey.Mock(pkgupdate.UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			return nil
		}).Build()

		tmpDir := t.TempDir()
		cliContext := newCLIContext(t, []string{
			"--next-version", "1.0.0",
			"--log-file", tmpDir + "/gpud.log",
		})
		err := Command(cliContext)
		require.NoError(t, err)
	})
}

// TestCommand_CustomURLOverridesDefault tests that a custom URL overrides the default.
func TestCommand_CustomURLOverridesDefault(t *testing.T) {
	mockey.PatchConvey("update command custom URL overrides default", t, func() {
		var capturedURL string
		mockey.Mock(pkgupdate.UpdateExecutable).To(func(targetVersion string, url string, requireRoot bool) error {
			capturedURL = url
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{"--next-version", "1.0.0", "--url", "https://mirror.example.com/"})
		err := Command(cliContext)
		require.NoError(t, err)
		assert.Equal(t, "https://mirror.example.com/", capturedURL)
		assert.NotEqual(t, version.DefaultURLPrefix, capturedURL)
	})
}

// =============================================================================
// CommandCheck Tests
// =============================================================================

// TestCommandCheck_InvalidLogLevel tests the check command with an invalid log level.
func TestCommandCheck_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("update check command invalid log level", t, func() {
		cliContext := newCLIContext(t, []string{"--log-level", "invalid-level"})
		err := CommandCheck(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unrecognized level")
	})
}

// TestCommandCheck_DetectLatestVersionError tests when auto-detecting the latest version fails.
func TestCommandCheck_DetectLatestVersionError(t *testing.T) {
	mockey.PatchConvey("update check command detect latest version error", t, func() {
		mockey.Mock(version.DetectLatestVersion).To(func() (string, error) {
			return "", errors.New("server unreachable")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandCheck(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "server unreachable")
	})
}

// TestCommandCheck_Success tests a successful version check.
func TestCommandCheck_Success(t *testing.T) {
	mockey.PatchConvey("update check command success", t, func() {
		mockey.Mock(version.DetectLatestVersion).To(func() (string, error) {
			return "3.0.0", nil
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandCheck(cliContext)
		require.NoError(t, err)
	})
}

// TestCommandCheck_ValidLogLevels tests the check command with various valid log levels.
func TestCommandCheck_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			mockey.PatchConvey("check valid log level "+level, t, func() {
				mockey.Mock(version.DetectLatestVersion).To(func() (string, error) {
					return "", errors.New("early exit")
				}).Build()

				cliContext := newCLIContext(t, []string{"--log-level", level})
				err := CommandCheck(cliContext)
				// Should fail at DetectLatestVersion, not at log level parsing
				require.Error(t, err)
				assert.Contains(t, err.Error(), "early exit")
			})
		})
	}
}

// TestCommandCheck_WithLogFile tests the check command with a log file flag.
func TestCommandCheck_WithLogFile(t *testing.T) {
	mockey.PatchConvey("update check command with log file", t, func() {
		mockey.Mock(version.DetectLatestVersion).To(func() (string, error) {
			return "3.0.0", nil
		}).Build()

		tmpDir := t.TempDir()
		cliContext := newCLIContext(t, []string{"--log-file", tmpDir + "/gpud.log"})
		err := CommandCheck(cliContext)
		require.NoError(t, err)
	})
}
