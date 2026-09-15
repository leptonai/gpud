// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package kapmtls

import (
	"context"
	"crypto/x509"
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
	runner.startAgent()
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
	assert.Contains(t, runner.calls, hupCommand, "idempotent retries must cover a crash before the previous HUP")
}

func TestStartupConfigurationDriftRejectedBeforeWriting(t *testing.T) {
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
		for _, active := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/active=%t", tc.name, active), func(t *testing.T) {
				m, runner, paths := newTestManager(t)
				initial := newTestCredentials(t, "worker-1", "machine-1", 1)
				require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
				if active {
					runner.startAgent()
				}
				before, err := m.currentReleaseID()
				require.NoError(t, err)
				runner.calls = nil
				next := renewedCredentials(t, initial, 2)
				tc.mutate(&next)
				require.ErrorContains(t, m.UpdateCredentials(context.Background(), "machine-1", next), "requires explicit maintenance/restart")
				current, err := m.currentReleaseID()
				require.NoError(t, err)
				assert.Equal(t, before, current)
				entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
				require.NoError(t, err)
				assert.Len(t, entries, 1, "rejected drift must not stage a generation")
				assert.Empty(t, runner.calls)
			})
		}
	}
}

func TestCrashSelectedLeafRetriesHUP(t *testing.T) {
	m, runner, _ := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	runner.startAgent()
	next := renewedCredentials(t, initial, 2)
	pending, err := m.stageCredentials("machine-1", next)
	require.NoError(t, err)
	require.NoError(t, m.swapCurrentSymlink(pending))

	status, err := m.Status(context.Background(), "machine-1")
	require.NoError(t, err)
	assert.False(t, status.CredentialsInstalled)
	assert.Equal(t, "2", status.CertificateSerial, "certificate fields describe persisted selection")
	runner.calls = nil
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", next))
	assert.Contains(t, runner.calls, hupCommand)
	assert.Equal(t, "2", runner.loaded.Load().serial)
}

func TestSameSerialAndExpiryReplacementCannotAcknowledgeReload(t *testing.T) {
	m, runner, paths := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	runner.startAgent()
	before, err := m.currentReleaseID()
	require.NoError(t, err)
	runner.calls = nil
	require.ErrorContains(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 1)), "distinct certificate serial or expiry")
	current, err := m.currentReleaseID()
	require.NoError(t, err)
	assert.Equal(t, before, current)
	entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Empty(t, runner.calls)
}

func TestFailedReloadCannotReuseRetainedCertificateIdentity(t *testing.T) {
	m, runner, paths := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	runner.startAgent()
	loaded, err := m.currentReleaseID()
	require.NoError(t, err)
	runner.killErr = errors.New("signal unavailable")
	require.ErrorContains(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 2)), "signal unavailable")
	selected, err := m.currentReleaseID()
	require.NoError(t, err)
	require.NotEqual(t, loaded, selected)
	assert.Equal(t, "1", runner.loaded.Load().serial)

	runner.killErr = nil
	runner.skipReload = true
	runner.calls = nil
	replacement := renewedCredentials(t, initial, 1)
	require.ErrorContains(t, m.UpdateCredentials(context.Background(), "machine-1", replacement), "distinct certificate serial or expiry")
	current, err := m.currentReleaseID()
	require.NoError(t, err)
	assert.Equal(t, selected, current)
	entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
	require.NoError(t, err)
	assert.Len(t, entries, 2, "rejected replacement must not stage or clean up retained generations")
	for _, id := range []string{loaded, selected} {
		_, err := m.inspectRelease("machine-1", id)
		require.NoError(t, err, "retained generations must remain complete and unchanged")
	}
	assert.Empty(t, runner.calls, "ambiguous replacement must fail before signaling")
}

func TestExpiredSelectedLeafCanRenewWithSameStartupConfig(t *testing.T) {
	m, runner, _ := newTestManager(t)
	initial := newTestCredentialsWithLeaf(t, "worker-1", "machine-1", 1, func(c *x509.Certificate) {
		c.NotAfter = testNow.Add(time.Hour)
	})
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	runner.startAgent()
	m.now = func() time.Time { return testNow.Add(2 * time.Hour) }
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", renewedCredentials(t, initial, 2)))
	assert.Equal(t, "2", runner.loaded.Load().serial)
}

func TestConflictingRetainedStartupConfigRequiresMaintenance(t *testing.T) {
	for _, loadedNewLeaf := range []bool{false, true} {
		for _, sameLeaf := range []bool{false, true} {
			t.Run(fmt.Sprintf("new-leaf-loaded=%t/same-leaf=%t", loadedNewLeaf, sameLeaf), func(t *testing.T) {
				m, runner, paths := newTestManager(t)
				initial := newTestCredentials(t, "worker-1", "machine-1", 1)
				require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
				runner.startAgent()
				next := renewedCredentials(t, initial, 2)
				if sameLeaf {
					next = initial
				}
				next.GatewayEndpoint = "kap.example.test:9443"
				pending, err := m.stageCredentials("machine-1", next)
				require.NoError(t, err)
				require.NoError(t, m.swapCurrentSymlink(pending))
				if loadedNewLeaf {
					// HUP reloads only the leaf, not the persisted startup settings.
					runner.loadSelectedCertificate()
				}
				runner.calls = nil
				_, err = m.Status(context.Background(), "machine-1")
				require.ErrorContains(t, err, "explicit maintenance/restart")
				require.ErrorContains(t, m.UpdateCredentials(context.Background(), "machine-1", next), "explicit maintenance/restart")
				require.ErrorContains(t, m.Activate(context.Background()), "explicit maintenance/restart")
				assert.Empty(t, runner.calls, "certificate metrics cannot resolve retained startup drift")
				current, err := m.currentReleaseID()
				require.NoError(t, err)
				assert.Equal(t, pending, current)
				entries, err := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
				require.NoError(t, err)
				assert.Len(t, entries, 2, "conflicting history is not cleaned away")
			})
		}
	}
}

func TestHotReloadFailuresRetainSelectionAndRetryWithoutRestart(t *testing.T) {
	for _, mode := range []string{"signal error", "stale metrics", "new serial wrong expiry", "canceled", "unready"} {
		t.Run(mode, func(t *testing.T) {
			m, runner, paths := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			runner.startAgent()
			previous, err := m.currentReleaseID()
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			signals := 0
			runner.afterSignal = func(context.Context) {
				assertPackageLocked(t)
				signals++
				if mode == "canceled" {
					cancel()
				}
				if mode == "new serial wrong expiry" {
					runner.loaded.Store(&runtimeCertificate{serial: "2", notAfter: testNow.Add(4 * 24 * time.Hour)})
				}
			}
			readyURL := m.readyURL
			switch mode {
			case "signal error":
				runner.killFailures = 1
			case "stale metrics", "new serial wrong expiry":
				runner.skipReload = true
			case "unready":
				m.readyURL = "http://127.0.0.1:1"
			}
			runner.calls = nil
			next := renewedCredentials(t, initial, 2)
			err = m.UpdateCredentials(ctx, "machine-1", next)
			require.Error(t, err)
			if mode == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			}
			current, readErr := m.currentReleaseID()
			require.NoError(t, readErr)
			assert.NotEqual(t, previous, current)
			assert.Equal(t, 1, signals, "there is no rollback signal")
			assert.NotContains(t, runner.calls, "systemctl restart "+AgentService)
			entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
			require.NoError(t, readErr)
			assert.Len(t, entries, 2, "failed forward update retains evidence")

			runner.afterSignal = nil
			runner.skipReload = false
			m.readyURL = readyURL
			runner.calls = nil
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", next))
			assert.Contains(t, runner.calls, hupCommand)
			assert.Equal(t, "2", runner.loaded.Load().serial)
			entries, err = os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
			require.NoError(t, err)
			assert.Len(t, entries, 1)
		})
	}
}

func TestSelectionFailureRetainsCompleteGenerationsWithoutSignaling(t *testing.T) {
	for _, mode := range []string{"before rename", "after rename"} {
		t.Run(mode, func(t *testing.T) {
			m, runner, paths := newTestManager(t)
			initial := newTestCredentials(t, "worker-1", "machine-1", 1)
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
			runner.startAgent()
			previous, err := m.currentReleaseID()
			require.NoError(t, err)
			next := renewedCredentials(t, initial, 2)
			nextID, err := m.stageCredentials("machine-1", next)
			require.NoError(t, err)
			selectionErr := errors.New("selection sync failed")
			blocked := filepath.Join(paths.StateDir, ".current-"+nextID)
			if mode == "before rename" {
				require.NoError(t, os.MkdirAll(filepath.Join(blocked, "keep"), 0700))
			} else {
				m.syncDir = func(string) error {
					assertPackageLocked(t)
					return selectionErr
				}
			}
			runner.calls = nil
			err = m.UpdateCredentials(context.Background(), "machine-1", next)
			require.Error(t, err)
			current, readErr := m.currentReleaseID()
			require.NoError(t, readErr)
			if mode == "before rename" {
				assert.Equal(t, previous, current)
				require.NoError(t, os.RemoveAll(blocked))
			} else {
				require.ErrorIs(t, err, selectionErr)
				assert.Equal(t, nextID, current, "a failed sync may leave the complete selection visible")
				require.ErrorIs(t, m.UpdateCredentials(context.Background(), "machine-1", next), selectionErr, "idempotent selection must retry the failed durability check")
			}
			assert.Equal(t, "1", runner.loaded.Load().serial)
			assert.NotContains(t, runner.calls, hupCommand)
			entries, readErr := os.ReadDir(filepath.Join(paths.StateDir, ReleasesDirectoryName))
			require.NoError(t, readErr)
			assert.Len(t, entries, 2)
			m.syncDir = syncDirectory
			require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", next))
			assert.Equal(t, "2", runner.loaded.Load().serial)
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
			runner.startAgent()
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
			assert.Equal(t, "1", status.CertificateSerial)
			assert.Equal(t, "kap.example.test:8443", status.GatewayEndpoint)
		})
	}
}

func TestRuntimeObservationRetriesTransientMetricReset(t *testing.T) {
	m, runner, _ := newTestManager(t)
	initial := newTestCredentials(t, "worker-1", "machine-1", 1)
	require.NoError(t, m.UpdateCredentials(context.Background(), "machine-1", initial))
	runner.startAgent()
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
	m.reloadTimeout = time.Second
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
