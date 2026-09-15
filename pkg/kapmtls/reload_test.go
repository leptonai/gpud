// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const hupCommand = "systemctl kill --signal=HUP --kill-who=main " + AgentService

func renewedCredentials(t *testing.T, previous Credentials, serial int64) Credentials {
	t.Helper()
	renewed := newTestCredentials(t, "worker-1", "machine-1", serial)
	renewed.GatewayCAPEM = previous.GatewayCAPEM
	renewed.GatewayCAFingerprint = previous.GatewayCAFingerprint
	renewed.ClientCAFingerprint = previous.ClientCAFingerprint
	renewed.GatewayEndpoint = previous.GatewayEndpoint
	renewed.ServerName = previous.ServerName
	return renewed
}

func metricText(serial string, notAfter time.Time) string {
	return fmt.Sprintf("# TYPE kaproxy_mtls_agent_cert_info gauge\nkaproxy_mtls_agent_cert_info{serial=%q} 1\n# TYPE kaproxy_mtls_agent_cert_not_after_timestamp_seconds gauge\nkaproxy_mtls_agent_cert_not_after_timestamp_seconds %d\n", serial, notAfter.Unix())
}

func TestLeafRenewalHotReloadsWithoutRestart(t *testing.T) {
	m, runner, paths := newTestManager(t)
	credentials := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", credentials))
	runner.calls = nil
	for serial := int64(2); serial <= 3; serial++ {
		credentials = renewedCredentials(t, credentials, serial)
		require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", credentials))
		status, err := m.Status(context.Background(), "machine-1")
		require.NoError(t, err)
		assert.True(t, status.CredentialsInstalled)
		assert.Equal(t, fmt.Sprintf("%x", serial), status.CertificateSerial)
	}
	assert.Equal(t, 2, strings.Count(strings.Join(runner.calls, "\n"), hupCommand))
	assert.NotContains(t, strings.Join(runner.calls, "\n"), "systemctl restart")
	assert.NotContains(t, strings.Join(runner.calls, "\n"), "systemctl enable")
	entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	runner.calls = nil
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", credentials))
	assert.NotContains(t, strings.Join(runner.calls, "\n"), hupCommand, "already loaded identical generation needs no signal")
}

func TestStartupConfigurationDriftRestarts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Credentials)
	}{
		{name: "gateway CA", mutate: func(c *Credentials) {
			other := newTestCredentials(t, "worker-1", "machine-1", 3)
			c.GatewayCAPEM = other.GatewayCAPEM
			c.GatewayCAFingerprint = other.GatewayCAFingerprint
		}},
		{name: "client CA", mutate: func(c *Credentials) { c.ClientCAFingerprint = strings.Repeat("a", sha256HexLength) }},
		{name: "endpoint port", mutate: func(c *Credentials) { c.GatewayEndpoint = "kap.example.test:9443" }},
		{name: "server name", mutate: func(c *Credentials) {
			c.GatewayEndpoint = "other.example.test:8443"
			c.ServerName = "other.example.test"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, runner, _ := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			runner.calls = nil
			next := renewedCredentials(t, initial, 2)
			tc.mutate(&next)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", next))
			assert.Contains(t, runner.calls, "systemctl restart "+AgentService)
			assert.NotContains(t, runner.calls, hupCommand)
		})
	}
}

func TestCrashSelectedGenerationUsesLoadedRelease(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(fmt.Sprintf("startup drift %t", drift), func(t *testing.T) {
			m, runner, _ := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			next := renewedCredentials(t, initial, 2)
			if drift {
				next.GatewayEndpoint = "kap.example.test:9443"
			}
			pending, err := m.stageCredentials("machine-1", next)
			require.NoError(t, err)
			require.NoError(t, m.swapCurrentSymlink(pending))
			status, err := m.Status(context.Background(), "machine-1")
			require.NoError(t, err)
			assert.False(t, status.CredentialsInstalled)
			assert.Empty(t, status.CertificateSerial, "disk selection is not runtime evidence")
			runner.calls = nil
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", next))
			if drift {
				assert.Contains(t, runner.calls, "systemctl restart "+AgentService)
				assert.NotContains(t, runner.calls, hupCommand)
			} else {
				assert.Contains(t, runner.calls, hupCommand)
				assert.NotContains(t, runner.calls, "systemctl restart "+AgentService)
			}
		})
	}
}

func TestCrashRestartFailureRestoresLoadedNotSelectedRelease(t *testing.T) {
	m, runner, _ := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	loaded, err := m.currentReleaseID()
	require.NoError(t, err)
	next := renewedCredentials(t, initial, 2)
	next.GatewayEndpoint = "kap.example.test:9443"
	pending, err := m.stageCredentials("machine-1", next)
	require.NoError(t, err)
	require.NoError(t, m.swapCurrentSymlink(pending))
	runner.restartFailures = 1
	require.ErrorContains(t, m.UpdateCredentials(context.Background(), "machine-1", next), "restart KAP mTLS agent")
	current, err := m.currentReleaseID()
	require.NoError(t, err)
	assert.Equal(t, loaded, current)
}

func TestAmbiguousLoadedGenerationUsesRestartRecovery(t *testing.T) {
	m, runner, _ := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	ambiguous := initial
	ambiguous.GatewayEndpoint = "kap.example.test:9443"
	pending, err := m.stageCredentials("machine-1", ambiguous)
	require.NoError(t, err)
	require.NoError(t, m.swapCurrentSymlink(pending))
	status, err := m.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.False(t, status.CredentialsInstalled)
	runner.calls = nil
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", ambiguous))
	assert.Contains(t, runner.calls, "systemctl restart "+AgentService)
	assert.NotContains(t, runner.calls, hupCommand)
}

func TestSameSerialDifferentGenerationCannotAcknowledgeHotReload(t *testing.T) {
	m, runner, _ := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	runner.calls = nil
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 1)))
	assert.Contains(t, runner.calls, "systemctl restart "+AgentService)
	assert.NotContains(t, runner.calls, hupCommand)
}

func TestHotReloadFailuresRestoreLoadedGenerationWithoutRestart(t *testing.T) {
	for _, mode := range []string{"signal error", "stale metrics", "new serial wrong expiry", "canceled", "rollback error"} {
		t.Run(mode, func(t *testing.T) {
			m, runner, paths := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			loaded, err := m.currentReleaseID()
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			signals := 0
			runner.afterSignal = func(signalCtx context.Context) {
				assertPackageLocked(t)
				signals++
				if mode == "canceled" && signals == 1 {
					cancel()
				}
				if mode == "new serial wrong expiry" {
					if signals == 1 {
						runner.loaded.Store(&runtimeCertificate{serial: "2", notAfter: testNow.Add(4 * 24 * time.Hour)})
					} else {
						runner.loadSelectedCertificate()
					}
				}
				if signals == 2 {
					assert.NoError(t, signalCtx.Err(), "rollback detaches from cancellation")
				}
			}
			switch mode {
			case "signal error":
				runner.killFailures = 1
			case "stale metrics", "new serial wrong expiry":
				runner.skipReload = true
			case "rollback error":
				runner.killErr = errors.New("signal unavailable")
			}
			runner.calls = nil
			err = m.UpdateCredentials(ctx, "machine-1", renewedCredentials(t, initial, 2))
			require.Error(t, err)
			if mode == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			}
			if mode == "rollback error" {
				require.ErrorContains(t, err, "restore loaded KAP mTLS agent certificate")
			}
			current, readErr := m.currentReleaseID()
			require.NoError(t, readErr)
			assert.Equal(t, loaded, current)
			assert.Equal(t, 2, signals)
			assert.NotContains(t, runner.calls, "systemctl restart "+AgentService)
			entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
			require.NoError(t, readErr)
			assert.Len(t, entries, 2, "failed forward update retains evidence")
		})
	}
}

func TestSelectionFailurePreservesLoadedGeneration(t *testing.T) {
	for _, mode := range []string{"before rename", "after rename", "rollback sync failure"} {
		t.Run(mode, func(t *testing.T) {
			m, runner, paths := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			loaded, err := m.currentReleaseID()
			require.NoError(t, err)
			next := renewedCredentials(t, initial, 2)
			nextID, err := m.stageCredentials("machine-1", next)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			selectionErr := errors.New("selection sync failed")
			rollbackErr := errors.New("rollback sync failed")
			syncs := 0
			if mode == "before rename" {
				blocked := filepath.Join(paths.StateDir, ".current-"+nextID)
				require.NoError(t, os.MkdirAll(filepath.Join(blocked, "keep"), 0700))
			} else {
				m.syncDir = func(path string) error {
					assertPackageLocked(t)
					syncs++
					if syncs == 1 {
						cancel()
						return selectionErr
					}
					if mode == "rollback sync failure" {
						return rollbackErr
					}
					return syncDirectory(path)
				}
			}
			signals := 0
			runner.afterSignal = func(signalCtx context.Context) {
				assertPackageLocked(t)
				assert.NoError(t, signalCtx.Err(), "rollback must detach from cancellation")
				signals++
			}
			runner.calls = nil
			err = m.UpdateCredentials(ctx, "machine-1", next)
			require.Error(t, err)
			if mode == "before rename" {
				assert.Zero(t, signals, "failed selection leaves the known process untouched")
			} else {
				require.ErrorIs(t, err, selectionErr)
				assert.Equal(t, 1, signals, "a visible rollback must reload even after a sync error")
				assert.Equal(t, 2, syncs)
			}
			if mode == "rollback sync failure" {
				require.ErrorIs(t, err, rollbackErr)
			}
			current, readErr := m.currentReleaseID()
			require.NoError(t, readErr)
			assert.Equal(t, loaded, current)
			assert.Equal(t, "1", runner.loaded.Load().serial)
			assert.NotContains(t, runner.calls, "systemctl restart "+AgentService)
			entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
			require.NoError(t, readErr)
			assert.Len(t, entries, 2)
		})
	}
}

func TestRuntimeCertificateMetricsValidation(t *testing.T) {
	valid := metricText("1", testNow.Add(5*24*time.Hour))
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{name: "valid", body: valid, valid: true},
		{name: "missing", body: ""},
		{name: "malformed", body: "not metrics"},
		{name: "missing expiry", body: strings.Split(valid, "# TYPE kaproxy_mtls_agent_cert_not_after")[0]},
		{name: "duplicate serial", body: valid + "kaproxy_mtls_agent_cert_info{serial=\"2\"} 1\n"},
		{name: "zero info", body: strings.Replace(valid, "} 1", "} 0", 1)},
		{name: "uppercase serial", body: strings.Replace(valid, `serial="1"`, `serial="A"`, 1)},
		{name: "noncanonical serial", body: strings.Replace(valid, `serial="1"`, `serial="01"`, 1)},
		{name: "extra label", body: strings.Replace(valid, `serial="1"`, `serial="1",other="x"`, 1)},
		{name: "oversized", body: strings.Repeat("# filler\n", maxMetricsBytes/8+1)},
		{name: "NaN expiry", body: strings.Replace(valid, fmt.Sprint(testNow.Add(5*24*time.Hour).Unix()), "NaN", 1)},
		{name: "infinite expiry", body: strings.Replace(valid, fmt.Sprint(testNow.Add(5*24*time.Hour).Unix()), "+Inf", 1)},
		{name: "fractional expiry", body: strings.Replace(valid, fmt.Sprint(testNow.Add(5*24*time.Hour).Unix()), "1.5", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _ := newTestManager(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/metrics", r.URL.Path)
				assert.Empty(t, r.URL.RawQuery)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			m.readyURL = server.URL + "/readyz?ignored=true"
			got, err := m.runtimeCertificate(context.Background())
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, "1", got.serial)
				assert.Equal(t, testNow.Add(5*24*time.Hour), got.notAfter)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestStatusMissingOrStaleRuntimeMetricsDoesNotClaimDiskCredentials(t *testing.T) {
	for _, body := range []string{"", "malformed", metricText("2", testNow.Add(5*24*time.Hour)), metricText("1", testNow.Add(4*24*time.Hour))} {
		t.Run(body, func(t *testing.T) {
			m, runner, _ := newTestManager(t)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 1)))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/metrics" {
					_, _ = w.Write([]byte(body))
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			m.readyURL = server.URL
			status, err := m.Status(context.Background(), "machine-1")
			require.NoError(t, err)
			assert.True(t, status.AgentActive)
			assert.True(t, status.AgentReady)
			assert.False(t, status.CredentialsInstalled)
			assert.Empty(t, status.CertificateSerial)
			runner.calls = nil
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", newTestCredentials(t, "worker-1", "machine-1", 2)))
			assert.Contains(t, runner.calls, "systemctl restart "+AgentService)
		})
	}
}

func TestRuntimeObservationRetriesTransientMetricReset(t *testing.T) {
	m, runner, _ := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertPackageLocked(t)
		if r.URL.Path == "/metrics" {
			if probes.Add(1) == 1 {
				return
			}
			loaded := runner.loaded.Load()
			_, _ = w.Write([]byte(metricText(loaded.serial, loaded.notAfter)))
		}
	}))
	defer server.Close()
	m.readyURL = server.URL
	m.runtimeTimeout = time.Second
	runner.calls = nil
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 2)))
	assert.Contains(t, runner.calls, hupCommand)
	assert.NotContains(t, runner.calls, "systemctl restart "+AgentService)
}

func TestRuntimeMetricsHonorsCanceledBodyRead(t *testing.T) {
	m, _, _ := newTestManager(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	m.readyURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := m.runtimeCertificate(ctx)
	require.Error(t, err)
}
