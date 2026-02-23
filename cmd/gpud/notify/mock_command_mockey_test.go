package notify

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"

	apiv1 "github.com/leptonai/gpud/api/v1"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// newCLIContext creates a CLI context for testing with the given arguments.
func newCLIContext(t *testing.T, args []string) *cli.Context {
	t.Helper()

	app := cli.NewApp()
	flags := flag.NewFlagSet("gpud-notify-test", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	_ = flags.String("log-level", "info", "")
	_ = flags.String("state-file", "", "")

	require.NoError(t, flags.Parse(args))
	return cli.NewContext(app, flags, nil)
}

// TestCommandStartup_InvalidLogLevel tests when an invalid log level is provided.
func TestCommandStartup_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("startup command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-notify-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("state-file", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandStartup(cliContext)
		require.Error(t, err)
	})
}

// TestCommandStartup_StateFileError tests when getting state file fails.
func TestCommandStartup_StateFileError(t *testing.T) {
	mockey.PatchConvey("startup command state file error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("failed to get state file")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandStartup(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get state file")
	})
}

// TestCommandStartup_SqliteOpenError tests when opening sqlite fails.
func TestCommandStartup_SqliteOpenError(t *testing.T) {
	mockey.PatchConvey("startup command sqlite open error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/some/state.db", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open database")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandStartup(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestCommandShutdown_InvalidLogLevel tests when an invalid log level is provided.
func TestCommandShutdown_InvalidLogLevel(t *testing.T) {
	mockey.PatchConvey("shutdown command invalid log level", t, func() {
		app := cli.NewApp()
		flags := flag.NewFlagSet("gpud-notify-test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)

		_ = flags.String("log-level", "invalid-level", "")
		_ = flags.String("state-file", "", "")

		require.NoError(t, flags.Parse([]string{"--log-level", "invalid-level"}))
		cliContext := cli.NewContext(app, flags, nil)

		err := CommandShutdown(cliContext)
		require.Error(t, err)
	})
}

// TestCommandShutdown_StateFileError tests when getting state file fails.
func TestCommandShutdown_StateFileError(t *testing.T) {
	mockey.PatchConvey("shutdown command state file error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "", errors.New("failed to get state file")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandShutdown(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get state file")
	})
}

// TestCommandShutdown_SqliteOpenError tests when opening sqlite fails.
func TestCommandShutdown_SqliteOpenError(t *testing.T) {
	mockey.PatchConvey("shutdown command sqlite open error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/some/state.db", nil
		}).Build()

		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return nil, errors.New("failed to open database")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandShutdown(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open state file")
	})
}

// TestCreateNotificationURL tests the createNotificationURL function.
func TestCreateNotificationURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		expected string
	}{
		{
			name:     "simple hostname",
			endpoint: "example.com",
			expected: "https://example.com/api/v1/notification",
		},
		{
			name:     "with https scheme",
			endpoint: "https://example.com",
			expected: "https://example.com/api/v1/notification",
		},
		{
			name:     "with http scheme",
			endpoint: "http://example.com",
			expected: "https://example.com/api/v1/notification",
		},
		{
			name:     "with port",
			endpoint: "https://example.com:8080",
			expected: "https://example.com:8080/api/v1/notification",
		},
		{
			name:     "with path",
			endpoint: "https://example.com/some/path",
			expected: "https://example.com/api/v1/notification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createNotificationURL(tt.endpoint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSendNotification_Success tests successful notification sending.
func TestSendNotification_Success(t *testing.T) {
	mockey.PatchConvey("send notification success", t, func() {
		// Create a test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-token")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		// Mock createNotificationURL to preserve HTTP scheme for testing
		mockey.Mock(createNotificationURL).To(func(endpoint string) string {
			return endpoint + "/api/v1/notification"
		}).Build()

		req := apiv1.NotificationRequest{
			ID:   "test-machine",
			Type: apiv1.NotificationTypeStartup,
		}
		err := sendNotification(server.URL, req, "test-token")
		require.NoError(t, err)
	})
}

// TestSendNotification_EmptyHost tests notification with empty host.
func TestSendNotification_EmptyHost(t *testing.T) {
	req := apiv1.NotificationRequest{
		ID:   "test-machine",
		Type: apiv1.NotificationTypeStartup,
	}
	err := sendNotification("/just/a/path", req, "test-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no host in endpoint URL")
}

// TestSendNotification_ServerError tests notification when server returns error.
func TestSendNotification_ServerError(t *testing.T) {
	mockey.PatchConvey("send notification server error", t, func() {
		// Create a test server that returns an error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "internal server error"}`))
		}))
		defer server.Close()

		// Mock createNotificationURL to preserve HTTP scheme for testing
		mockey.Mock(createNotificationURL).To(func(endpoint string) string {
			return endpoint + "/api/v1/notification"
		}).Build()

		req := apiv1.NotificationRequest{
			ID:   "test-machine",
			Type: apiv1.NotificationTypeStartup,
		}
		err := sendNotification(server.URL, req, "test-token")
		require.Error(t, err)
	})
}

// TestSendNotification_ServerBadRequest tests notification when server returns bad request.
func TestSendNotification_ServerBadRequest(t *testing.T) {
	mockey.PatchConvey("send notification server bad request", t, func() {
		// Create a test server that returns a bad request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "bad request", "message": "invalid machine ID"}`))
		}))
		defer server.Close()

		// Mock createNotificationURL to preserve HTTP scheme for testing
		mockey.Mock(createNotificationURL).To(func(endpoint string) string {
			return endpoint + "/api/v1/notification"
		}).Build()

		req := apiv1.NotificationRequest{
			ID:   "test-machine",
			Type: apiv1.NotificationTypeStartup,
		}
		err := sendNotification(server.URL, req, "test-token")
		require.Error(t, err)
	})
}

// TestSendNotification_InvalidJSONResponse tests notification when server returns invalid JSON.
func TestSendNotification_InvalidJSONResponse(t *testing.T) {
	mockey.PatchConvey("send notification invalid JSON response", t, func() {
		// Create a test server that returns invalid JSON
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`not valid json`))
		}))
		defer server.Close()

		// Mock createNotificationURL to preserve HTTP scheme for testing
		mockey.Mock(createNotificationURL).To(func(endpoint string) string {
			return endpoint + "/api/v1/notification"
		}).Build()

		req := apiv1.NotificationRequest{
			ID:   "test-machine",
			Type: apiv1.NotificationTypeStartup,
		}
		err := sendNotification(server.URL, req, "test-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parsing error response")
	})
}

// TestCommandStartup_ValidLogLevels tests that valid log levels are accepted.
func TestCommandStartup_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			mockey.PatchConvey("startup valid log level "+level, t, func() {
				mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
					return "", errors.New("early exit")
				}).Build()

				app := cli.NewApp()
				flags := flag.NewFlagSet("gpud-notify-test", flag.ContinueOnError)
				flags.SetOutput(io.Discard)

				_ = flags.String("log-level", level, "")
				_ = flags.String("state-file", "", "")

				require.NoError(t, flags.Parse([]string{"--log-level", level}))
				cliContext := cli.NewContext(app, flags, nil)

				// Will fail on state file, but log level parsing should succeed
				err := CommandStartup(cliContext)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "early exit")
			})
		})
	}
}

// TestCommandShutdown_ValidLogLevels tests that valid log levels are accepted.
func TestCommandShutdown_ValidLogLevels(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error"}

	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			mockey.PatchConvey("shutdown valid log level "+level, t, func() {
				mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
					return "", errors.New("early exit")
				}).Build()

				app := cli.NewApp()
				flags := flag.NewFlagSet("gpud-notify-test", flag.ContinueOnError)
				flags.SetOutput(io.Discard)

				_ = flags.String("log-level", level, "")
				_ = flags.String("state-file", "", "")

				require.NoError(t, flags.Parse([]string{"--log-level", level}))
				cliContext := cli.NewContext(app, flags, nil)

				// Will fail on state file, but log level parsing should succeed
				err := CommandShutdown(cliContext)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "early exit")
			})
		})
	}
}

// =============================================================================
// Deeper Flow Tests - CommandStartup
// =============================================================================

// TestCommandStartup_ReadMachineIDError tests when ReadMachineID returns an error.
func TestCommandStartup_ReadMachineIDError(t *testing.T) {
	mockey.PatchConvey("startup command read machine ID error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("failed to read machine ID from db")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandStartup(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read machine ID")
	})
}

// TestCommandStartup_ReadEndpointError tests when ReadMetadata for endpoint returns an error.
func TestCommandStartup_ReadEndpointError(t *testing.T) {
	mockey.PatchConvey("startup command read endpoint error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", errors.New("failed to read endpoint metadata")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandStartup(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read endpoint")
	})
}

// TestCommandStartup_EmptyEndpoint tests when endpoint is empty (triggers os.Exit).
func TestCommandStartup_EmptyEndpoint(t *testing.T) {
	mockey.PatchConvey("startup command empty endpoint", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil // empty endpoint
		}).Build()

		exitCalled := false
		exitCode := -1
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			exitCode = code
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit mock
				_ = r
			}
		}()

		_ = CommandStartup(cliContext)

		assert.True(t, exitCalled, "expected os.Exit to be called for empty endpoint")
		assert.Equal(t, 0, exitCode)
	})
}

// TestCommandStartup_EmptyToken tests when token is empty (triggers os.Exit).
func TestCommandStartup_EmptyToken(t *testing.T) {
	mockey.PatchConvey("startup command empty token", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "test-endpoint.com", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", nil // empty token
		}).Build()

		exitCalled := false
		exitCode := -1
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			exitCode = code
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit mock
				_ = r
			}
		}()

		_ = CommandStartup(cliContext)

		assert.True(t, exitCalled, "expected os.Exit to be called for empty token")
		assert.Equal(t, 0, exitCode)
	})
}

// TestCommandStartup_SendNotificationSuccess tests the full startup flow through to sendNotification.
func TestCommandStartup_SendNotificationSuccess(t *testing.T) {
	mockey.PatchConvey("startup command send notification success", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "test-endpoint.com", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-token", nil
		}).Build()

		var capturedEndpoint string
		var capturedReqType apiv1.NotificationType
		var capturedReqID string
		var capturedToken string
		mockey.Mock(sendNotification).To(func(endpoint string, req apiv1.NotificationRequest, token string) error {
			capturedEndpoint = endpoint
			capturedReqType = req.Type
			capturedReqID = req.ID
			capturedToken = token
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandStartup(cliContext)
		require.NoError(t, err)
		assert.Equal(t, "test-endpoint.com", capturedEndpoint)
		assert.Equal(t, apiv1.NotificationTypeStartup, capturedReqType)
		assert.Equal(t, "test-machine-id", capturedReqID)
		assert.Equal(t, "test-token", capturedToken)
	})
}

// TestCommandStartup_SendNotificationError tests when sendNotification returns an error.
func TestCommandStartup_SendNotificationError(t *testing.T) {
	mockey.PatchConvey("startup command send notification error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "test-endpoint.com", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-token", nil
		}).Build()
		mockey.Mock(sendNotification).To(func(endpoint string, req apiv1.NotificationRequest, token string) error {
			return errors.New("connection refused")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandStartup(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection refused")
	})
}

// =============================================================================
// Deeper Flow Tests - CommandShutdown
// =============================================================================

// TestCommandShutdown_ReadMachineIDError tests when ReadMachineID returns an error.
func TestCommandShutdown_ReadMachineIDError(t *testing.T) {
	mockey.PatchConvey("shutdown command read machine ID error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("failed to read machine ID from db")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandShutdown(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read machine ID")
	})
}

// TestCommandShutdown_ReadEndpointError tests when ReadMetadata for endpoint returns an error.
func TestCommandShutdown_ReadEndpointError(t *testing.T) {
	mockey.PatchConvey("shutdown command read endpoint error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", errors.New("failed to read endpoint metadata")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandShutdown(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read endpoint")
	})
}

// TestCommandShutdown_EmptyEndpoint tests when endpoint is empty (triggers os.Exit).
func TestCommandShutdown_EmptyEndpoint(t *testing.T) {
	mockey.PatchConvey("shutdown command empty endpoint", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "", nil // empty endpoint
		}).Build()

		exitCalled := false
		exitCode := -1
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			exitCode = code
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit mock
				_ = r
			}
		}()

		_ = CommandShutdown(cliContext)

		assert.True(t, exitCalled, "expected os.Exit to be called for empty endpoint")
		assert.Equal(t, 0, exitCode)
	})
}

// TestCommandShutdown_SendNotificationSuccess tests the full shutdown flow through to sendNotification.
func TestCommandShutdown_SendNotificationSuccess(t *testing.T) {
	mockey.PatchConvey("shutdown command send notification success", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "test-endpoint.com", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-token", nil
		}).Build()

		var capturedReqType apiv1.NotificationType
		mockey.Mock(sendNotification).To(func(endpoint string, req apiv1.NotificationRequest, token string) error {
			capturedReqType = req.Type
			return nil
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandShutdown(cliContext)
		require.NoError(t, err)
		assert.Equal(t, apiv1.NotificationTypeShutdown, capturedReqType)
	})
}

// TestCommandShutdown_SendNotificationError tests when sendNotification returns an error for shutdown.
func TestCommandShutdown_SendNotificationError(t *testing.T) {
	mockey.PatchConvey("shutdown command send notification error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "test-endpoint.com", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-token", nil
		}).Build()
		mockey.Mock(sendNotification).To(func(endpoint string, req apiv1.NotificationRequest, token string) error {
			return errors.New("connection timeout")
		}).Build()

		cliContext := newCLIContext(t, []string{})
		err := CommandShutdown(cliContext)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection timeout")
	})
}

// TestCommandStartup_ReadTokenError tests when ReadToken returns an error (triggers os.Exit).
func TestCommandStartup_ReadTokenError(t *testing.T) {
	mockey.PatchConvey("startup command read token error", t, func() {
		mockey.Mock(gpudcommon.StateFileFromContext).To(func(cliContext *cli.Context) (string, error) {
			return "/tmp/test.state", nil
		}).Build()
		mockey.Mock(sqlite.Open).To(func(dbPath string, opts ...sqlite.OpOption) (*sql.DB, error) {
			return &sql.DB{}, nil
		}).Build()
		mockey.Mock((*sql.DB).Close).To(func(*sql.DB) error {
			return nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMachineID).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "test-machine-id", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadMetadata).To(func(ctx context.Context, db *sql.DB, key string) (string, error) {
			return "test-endpoint.com", nil
		}).Build()
		mockey.Mock(pkgmetadata.ReadToken).To(func(ctx context.Context, db *sql.DB) (string, error) {
			return "", errors.New("token read error")
		}).Build()

		exitCalled := false
		mockey.Mock(os.Exit).To(func(code int) {
			exitCalled = true
			panic("os.Exit called")
		}).Build()

		cliContext := newCLIContext(t, []string{})

		defer func() {
			if r := recover(); r != nil {
				// Expected panic from os.Exit mock
				_ = r
			}
		}()

		_ = CommandStartup(cliContext)

		// ReadToken error also triggers os.Exit(0) due to "err != nil || dbToken == ''"
		assert.True(t, exitCalled, "expected os.Exit to be called for token read error")
	})
}
