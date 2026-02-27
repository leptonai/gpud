package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	lepconfig "github.com/leptonai/gpud/pkg/config"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgmetricsstore "github.com/leptonai/gpud/pkg/metrics/store"
	pkgmetricssyncer "github.com/leptonai/gpud/pkg/metrics/syncer"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia/nvml/lib"
	pkgsession "github.com/leptonai/gpud/pkg/session"
)

type nonClosableComponent struct {
	name string
}

func (c *nonClosableComponent) Name() string { return c.name }
func (c *nonClosableComponent) Tags() []string {
	return []string{c.name}
}
func (c *nonClosableComponent) IsSupported() bool { return true }
func (c *nonClosableComponent) Start() error      { return nil }
func (c *nonClosableComponent) Check() components.CheckResult {
	return &mockCheckResult{
		componentName:   c.name,
		healthStateType: apiv1.HealthStateTypeHealthy,
		summary:         "ok",
	}
}
func (c *nonClosableComponent) Events(context.Context, time.Time) (apiv1.Events, error) {
	return nil, nil
}
func (c *nonClosableComponent) LastHealthStates() apiv1.HealthStates { return nil }
func (c *nonClosableComponent) Close() error                         { return nil }

type rebootStoreWithCloseError struct{}

func (r *rebootStoreWithCloseError) RecordReboot(context.Context) error { return nil }
func (r *rebootStoreWithCloseError) GetRebootEvents(context.Context, time.Time) (eventstore.Events, error) {
	return nil, nil
}
func (r *rebootStoreWithCloseError) Close() error { return errors.New("close reboot store failed") }

func TestNew_SuccessPathWithPprofAndPlugins_WithMockey(t *testing.T) {
	mockey.PatchConvey("New covers pprof, plugin specs, init checks, and goroutine startup", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-server-new-success-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		specsPath := filepath.Join(tmpDir, "plugins.yaml")
		require.NoError(t, os.WriteFile(specsPath, []byte("stub"), 0o644))

		t.Setenv(nvmllib.EnvMockAllSuccess, "true")

		var updateTokenCalled atomic.Bool
		var startListenerCalled atomic.Bool
		var versionFileCalled atomic.Bool

		mockey.Mock((*Server).updateToken).To(func(_ *Server, _ context.Context, _ pkgmetrics.Store, _ *UserToken) {
			updateTokenCalled.Store(true)
		}).Build()
		mockey.Mock((*Server).startListener).To(func(_ *Server, _ nvidianvml.Instance, _ *pkgmetricssyncer.Syncer, _ *lepconfig.Config, _ *gin.Engine, _ tls.Certificate) {
			startListenerCalled.Store(true)
		}).Build()
		mockey.Mock(updateFromVersionFile).To(func(_ context.Context, _ int, _ string) {
			versionFileCalled.Store(true)
		}).Build()

		mockey.Mock(pkgcustomplugins.LoadSpecs).To(func(path string) (pkgcustomplugins.Specs, error) {
			if path != specsPath {
				return nil, errors.New("unexpected specs path")
			}
			return pkgcustomplugins.Specs{
				{
					PluginName: "init-plugin",
					PluginType: pkgcustomplugins.SpecTypeInit,
					RunMode:    string(apiv1.RunModeTypeAuto),
				},
				{
					PluginName: "component-plugin",
					PluginType: pkgcustomplugins.SpecTypeComponent,
					RunMode:    string(apiv1.RunModeTypeManual),
				},
			}, nil
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg := &lepconfig.Config{
			Address:                "127.0.0.1:8443",
			DataDir:                tmpDir,
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
			EventsRetentionPeriod:  metav1.Duration{Duration: time.Minute},
			Components:             []string{"-disable-all"},
			DBInMemory:             true,
			SessionToken:           "session-token",
			SessionMachineID:       "session-machine-id",
			SessionEndpoint:        "https://api.example.com",
			PluginSpecsFile:        specsPath,
			Pprof:                  true,
			AutoUpdateExitCode:     0,
			VersionFile:            filepath.Join(tmpDir, "version-file"),
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		require.NoError(t, err)
		require.NotNil(t, s)

		require.Eventually(t, func() bool {
			return updateTokenCalled.Load() && startListenerCalled.Load() && versionFileCalled.Load()
		}, 2*time.Second, 20*time.Millisecond)

		s.Stop()
	})
}

func TestNew_SeedMetadataErrorBranches_WithMockey(t *testing.T) {
	tests := []struct {
		name       string
		failKey    string
		expectText string
	}{
		{
			name:       "token seed error",
			failKey:    pkgmetadata.MetadataKeyToken,
			expectText: "failed to seed session token",
		},
		{
			name:       "machine id seed error",
			failKey:    pkgmetadata.MetadataKeyMachineID,
			expectText: "failed to seed machine ID",
		},
		{
			name:       "endpoint seed error",
			failKey:    pkgmetadata.MetadataKeyEndpoint,
			expectText: "failed to seed endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockey.PatchConvey(tt.name, t, func() {
				tmpDir, err := os.MkdirTemp("", "gpud-server-seed-*")
				require.NoError(t, err)
				defer func() { _ = os.RemoveAll(tmpDir) }()

				t.Setenv(nvmllib.EnvMockAllSuccess, "true")

				mockey.Mock(pkgmetadata.SetMetadata).To(func(_ context.Context, _ *sql.DB, key, value string) error {
					if key == tt.failKey {
						return errors.New("seed failed")
					}
					return nil
				}).Build()

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				cfg := &lepconfig.Config{
					Address:                "127.0.0.1:8443",
					DataDir:                tmpDir,
					MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
					Components:             []string{"-disable-all"},
					DBInMemory:             true,
					SessionToken:           "session-token",
					SessionMachineID:       "session-machine-id",
					SessionEndpoint:        "https://api.example.com",
				}

				s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
				require.Error(t, err)
				require.Nil(t, s)
				assert.Contains(t, err.Error(), tt.expectText)
			})
		})
	}
}

func TestNew_MetricsStoreCreationError_WithMockey(t *testing.T) {
	mockey.PatchConvey("New returns error when creating metrics store fails", t, func() {
		tmpDir, err := os.MkdirTemp("", "gpud-server-metrics-store-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tmpDir) }()

		t.Setenv(nvmllib.EnvMockAllSuccess, "true")

		mockey.Mock(pkgmetricsstore.NewSQLiteStore).To(func(_ context.Context, _ *sql.DB, _ *sql.DB, _ string) (pkgmetrics.Store, error) {
			return nil, errors.New("metrics store failed")
		}).Build()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cfg := &lepconfig.Config{
			Address:                "127.0.0.1:8443",
			DataDir:                tmpDir,
			MetricsRetentionPeriod: metav1.Duration{Duration: time.Minute},
			Components:             []string{"-disable-all"},
			DBInMemory:             true,
			SessionToken:           "session-token",
			SessionMachineID:       "session-machine-id",
			SessionEndpoint:        "https://api.example.com",
		}

		s, err := New(ctx, log.NewNopAuditLogger(), cfg, nil)
		require.Error(t, err)
		require.Nil(t, s)
		assert.Contains(t, err.Error(), "failed to create metrics store")
	})
}

func TestUpdateToken_PipeReadAndSessionRecreateBranches_WithMockey(t *testing.T) {
	mockey.PatchConvey("updateToken reads token from pipe, restarts session, and handles session create error", t, func() {
		s := &Server{
			machineID:         "machine-1",
			epLocalGPUdServer: "https://local",
			epControlPlane:    "https://control",
			fifoPath:          "/tmp/gpud-fifo-test",
			gpudInstance:      &components.GPUdInstance{},
			session:           &pkgsession.Session{},
		}
		userToken := &UserToken{}

		r, w, err := os.Pipe()
		require.NoError(t, err)
		_, err = w.Write([]byte("pipe-token"))
		require.NoError(t, err)
		_ = w.Close()

		mockey.Mock(pkgmetadata.ReadToken).To(func(_ context.Context, _ *sql.DB) (string, error) {
			return "", errors.New("no db token")
		}).Build()
		mockey.Mock(os.Stat).To(func(_ string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}).Build()
		mockey.Mock(syscall.Mkfifo).To(func(_ string, _ uint32) error {
			return nil
		}).Build()

		openCalls := 0
		mockey.Mock(os.OpenFile).To(func(_ string, _ int, _ os.FileMode) (*os.File, error) {
			openCalls++
			if openCalls == 1 {
				return r, nil
			}
			return nil, errors.New("open failed")
		}).Build()

		var sessionStopCalled atomic.Bool
		mockey.Mock((*pkgsession.Session).Stop).To(func(_ *pkgsession.Session) {
			sessionStopCalled.Store(true)
		}).Build()

		var newSessionCalls atomic.Int32
		mockey.Mock(pkgsession.NewSession).To(func(_ context.Context, _, _, _ string, _ ...pkgsession.OpOption) (*pkgsession.Session, error) {
			newSessionCalls.Add(1)
			return nil, errors.New("session create failed")
		}).Build()

		mockey.Mock(time.Sleep).To(func(_ time.Duration) {}).Build()

		s.updateToken(context.Background(), nil, userToken)

		userToken.mu.RLock()
		defer userToken.mu.RUnlock()
		assert.Equal(t, "pipe-token", userToken.userToken)
		assert.True(t, sessionStopCalled.Load())
		assert.Equal(t, int32(1), newSessionCalls.Load())
	})
}

func TestUpdateToken_PipeReadErrorAndStatErrorBranches_WithMockey(t *testing.T) {
	t.Run("pipe read error", func(t *testing.T) {
		mockey.PatchConvey("updateToken handles pipe read error path", t, func() {
			s := &Server{
				machineID:         "machine-1",
				epLocalGPUdServer: "https://local",
				epControlPlane:    "https://control",
				fifoPath:          "/tmp/gpud-fifo-test",
				gpudInstance:      &components.GPUdInstance{},
			}
			userToken := &UserToken{}

			_, w, err := os.Pipe()
			require.NoError(t, err)

			mockey.Mock(pkgmetadata.ReadToken).To(func(_ context.Context, _ *sql.DB) (string, error) {
				return "", errors.New("no db token")
			}).Build()
			mockey.Mock(os.Stat).To(func(_ string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			}).Build()
			mockey.Mock(syscall.Mkfifo).To(func(_ string, _ uint32) error {
				return nil
			}).Build()

			openCalls := 0
			mockey.Mock(os.OpenFile).To(func(_ string, _ int, _ os.FileMode) (*os.File, error) {
				openCalls++
				if openCalls == 1 {
					return w, nil
				}
				return nil, errors.New("open failed")
			}).Build()

			mockey.Mock(time.Sleep).To(func(_ time.Duration) {}).Build()

			s.updateToken(context.Background(), nil, userToken)

			userToken.mu.RLock()
			defer userToken.mu.RUnlock()
			assert.Equal(t, "", userToken.userToken)
		})
	})

	t.Run("stat error", func(t *testing.T) {
		mockey.PatchConvey("updateToken returns when stat returns non-not-exist error", t, func() {
			s := &Server{
				fifoPath:     "/tmp/gpud-fifo-test",
				gpudInstance: &components.GPUdInstance{},
			}
			userToken := &UserToken{}

			mockey.Mock(pkgmetadata.ReadToken).To(func(_ context.Context, _ *sql.DB) (string, error) {
				return "", errors.New("no db token")
			}).Build()
			mockey.Mock(os.Stat).To(func(_ string) (os.FileInfo, error) {
				return nil, errors.New("stat failed")
			}).Build()

			s.updateToken(context.Background(), nil, userToken)
		})
	})
}

func TestServerStop_Branches_WithMockey(t *testing.T) {
	mockey.PatchConvey("Stop handles session stop, reboot store close error, and fifo close error", t, func() {
		registry := newMockRegistry()
		registry.AddMockComponent(&nonClosableComponent{name: "component"})

		f, err := os.CreateTemp("", "gpud-stop-fifo-*")
		require.NoError(t, err)
		fifoPath := f.Name()
		require.NoError(t, f.Close())
		defer func() { _ = os.Remove(fifoPath) }()

		var sessionStopCalled atomic.Bool
		mockey.Mock((*pkgsession.Session).Stop).To(func(_ *pkgsession.Session) {
			sessionStopCalled.Store(true)
		}).Build()

		s := &Server{
			session:            &pkgsession.Session{},
			componentsRegistry: registry,
			gpudInstance: &components.GPUdInstance{
				RebootEventStore: &rebootStoreWithCloseError{},
			},
			fifo:     f,
			fifoPath: "",
		}

		s.Stop()
		assert.True(t, sessionStopCalled.Load())
	})
}

func TestGenerateSelfSignedCert_ErrorBranches_WithMockey(t *testing.T) {
	t.Run("generate key error", func(t *testing.T) {
		mockey.PatchConvey("generateSelfSignedCert returns key generation error", t, func() {
			mockey.Mock(ecdsa.GenerateKey).To(func(_ elliptic.Curve, _ io.Reader) (*ecdsa.PrivateKey, error) {
				return nil, errors.New("generate key failed")
			}).Build()

			s := &Server{}
			_, err := s.generateSelfSignedCert()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "generate key failed")
		})
	})

	t.Run("create certificate error", func(t *testing.T) {
		mockey.PatchConvey("generateSelfSignedCert returns certificate creation error", t, func() {
			mockey.Mock(x509.CreateCertificate).To(func(_ io.Reader, _ *x509.Certificate, _ *x509.Certificate, _ any, _ any) ([]byte, error) {
				return nil, errors.New("create cert failed")
			}).Build()

			s := &Server{}
			_, err := s.generateSelfSignedCert()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "create cert failed")
		})
	})

	t.Run("marshal private key error", func(t *testing.T) {
		mockey.PatchConvey("generateSelfSignedCert returns private key marshal error", t, func() {
			mockey.Mock(x509.MarshalECPrivateKey).To(func(_ *ecdsa.PrivateKey) ([]byte, error) {
				return nil, errors.New("marshal private key failed")
			}).Build()

			s := &Server{}
			_, err := s.generateSelfSignedCert()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "marshal private key failed")
		})
	})

	t.Run("x509 key pair error", func(t *testing.T) {
		mockey.PatchConvey("generateSelfSignedCert returns x509 key pair error", t, func() {
			mockey.Mock(tls.X509KeyPair).To(func(_, _ []byte) (tls.Certificate, error) {
				return tls.Certificate{}, errors.New("x509 key pair failed")
			}).Build()

			s := &Server{}
			_, err := s.generateSelfSignedCert()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "x509 key pair failed")
		})
	})
}

func TestWriteToken_StatErrorBranch_WithMockey(t *testing.T) {
	mockey.PatchConvey("WriteToken returns stat error for non-not-exist failure", t, func() {
		mockey.Mock(os.Stat).To(func(_ string) (os.FileInfo, error) {
			return nil, errors.New("stat failed")
		}).Build()

		err := WriteToken("token", "/tmp/non-existent-fifo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to stat fifo file")
	})
}
