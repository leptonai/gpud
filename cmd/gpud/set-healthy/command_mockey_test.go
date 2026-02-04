package sethealthy

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	clientv1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/pkg/config"
)

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-set-healthy-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("log-file", "", "")
	_ = flags.String("server", "", "")

	require.NoError(t, flags.Parse(args))

	return cli.NewContext(app, flags, nil)
}

// captureStdout captures stdout during function execution.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	done := make(chan string, 1)
	go func(reader *os.File) {
		b, _ := io.ReadAll(reader)
		done <- string(b)
	}(r)

	fn()

	require.NoError(t, w.Close())
	os.Stdout = oldStdout

	out := <-done
	require.NoError(t, r.Close())
	return out
}

// TestCreateCommand_InvalidLogLevel tests the command with an invalid log level.
func TestCreateCommand_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-set-healthy-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("log-file", "", "")
		_ = flags.String("server", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		cmd := CreateCommand()
		err := cmd(cliContext)
		require.Error(t, err)
	})
}

// TestCreateCommand_Success tests successful component healthy setting.
func TestCreateCommand_Success(t *testing.T) {
	mockey.PatchConvey("success", t, func() {
		mockey.Mock(clientv1.SetHealthyComponents).To(func(ctx context.Context, addr string, components []string, opts ...clientv1.OpOption) ([]string, error) {
			return components, nil
		}).Build()

		cliContext := newCLIContext(t, []string{})

		cmd := CreateCommand()
		output := captureStdout(t, func() {
			err := cmd(cliContext)
			require.NoError(t, err)
		})

		assert.Contains(t, output, "successfully set components to healthy")
	})
}

// TestCreateCommand_WithServerFlag tests the command with a custom server address.
func TestCreateCommand_WithServerFlag(t *testing.T) {
	mockey.PatchConvey("with server flag", t, func() {
		var capturedAddr string
		mockey.Mock(clientv1.SetHealthyComponents).To(func(ctx context.Context, addr string, components []string, opts ...clientv1.OpOption) ([]string, error) {
			capturedAddr = addr
			return components, nil
		}).Build()

		cliContext := newCLIContext(t, []string{"--server", "https://custom-server:8080"})

		cmd := CreateCommand()
		_ = captureStdout(t, func() {
			err := cmd(cliContext)
			require.NoError(t, err)
		})

		assert.Equal(t, "https://custom-server:8080", capturedAddr)
	})
}

// TestCreateCommand_DefaultServerAddress tests the command uses default server address.
func TestCreateCommand_DefaultServerAddress(t *testing.T) {
	mockey.PatchConvey("default server address", t, func() {
		var capturedAddr string
		mockey.Mock(clientv1.SetHealthyComponents).To(func(ctx context.Context, addr string, components []string, opts ...clientv1.OpOption) ([]string, error) {
			capturedAddr = addr
			return components, nil
		}).Build()

		cliContext := newCLIContext(t, []string{})

		cmd := CreateCommand()
		_ = captureStdout(t, func() {
			err := cmd(cliContext)
			require.NoError(t, err)
		})

		expectedAddr := "https://localhost:" + itoa(config.DefaultGPUdPort)
		assert.Equal(t, expectedAddr, capturedAddr)
	})
}

// itoa converts int to string using fmt
func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

// TestCreateCommand_SetHealthyError tests error handling when SetHealthyComponents fails.
func TestCreateCommand_SetHealthyError(t *testing.T) {
	mockey.PatchConvey("set healthy error", t, func() {
		mockey.Mock(clientv1.SetHealthyComponents).To(func(ctx context.Context, addr string, components []string, opts ...clientv1.OpOption) ([]string, error) {
			return nil, errors.New("connection refused")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		cmd := CreateCommand()
		err := cmd(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set components healthy")
		assert.Contains(t, err.Error(), "connection refused")
	})
}

// TestCreateCommand_ValidLogLevels tests that valid log levels are accepted.
func TestCreateCommand_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn"}

	for _, level := range validLevels {
		t.Run("level_"+level, func(t *testing.T) {
			mockey.PatchConvey("valid log level "+level, t, func() {
				mockey.Mock(clientv1.SetHealthyComponents).To(func(ctx context.Context, addr string, components []string, opts ...clientv1.OpOption) ([]string, error) {
					return components, nil
				}).Build()

				cliContext := newCLIContext(t, []string{"--log-level", level})

				cmd := CreateCommand()
				err := cmd(cliContext)
				require.NoError(t, err)
			})
		})
	}
}

// TestCreateCommand_ReturnsFunction tests that CreateCommand returns a function.
func TestCreateCommand_ReturnsFunction(t *testing.T) {
	cmd := CreateCommand()
	assert.NotNil(t, cmd)
}
