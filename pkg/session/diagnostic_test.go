package session

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/log"
)

func TestDiagnosticRequestResponseJSON(t *testing.T) {
	payload := Request{
		Method: "diagnostic",
		Diagnostic: &DiagnosticRequest{
			ReportID:       "diag_1",
			Type:           diagnosticTypeNvidiaBugReport,
			TimeoutSeconds: 600,
			MaxSizeBytes:   128 << 20,
			ChunkSizeBytes: 1 << 20,
		},
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"method":"diagnostic"`)
	assert.Contains(t, string(raw), `"type":"nvidia_bug_report"`)

	var decoded Request
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotNil(t, decoded.Diagnostic)
	assert.Equal(t, "diag_1", decoded.Diagnostic.ReportID)
	assert.Equal(t, int64(600), decoded.Diagnostic.TimeoutSeconds)

	resp := Response{Diagnostic: &DiagnosticAcceptedResponse{Accepted: true}}
	raw, err = json.Marshal(resp)
	require.NoError(t, err)
	assert.JSONEq(t, `{"diagnostic":{"accepted":true}}`, string(raw))
}

func TestProcessDiagnosticRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name    string
		session *Session
		req     *DiagnosticRequest
		reason  string
	}{
		{
			name:    "missing request",
			session: diagnosticTestSession(t, nil),
			req:     nil,
			reason:  "missing_diagnostic_request",
		},
		{
			name:    "missing report id",
			session: diagnosticTestSession(t, nil),
			req:     &DiagnosticRequest{Type: diagnosticTypeNvidiaBugReport},
			reason:  "missing_report_id",
		},
		{
			name:    "unsupported type",
			session: diagnosticTestSession(t, nil),
			req:     &DiagnosticRequest{ReportID: "diag_1", Type: "shell"},
			reason:  "unsupported_diagnostic_type",
		},
		{
			name:    "missing endpoint",
			session: &Session{ctx: context.Background(), token: "token"},
			req:     &DiagnosticRequest{ReportID: "diag_1", Type: diagnosticTypeNvidiaBugReport},
			reason:  "missing_control_plane_endpoint",
		},
		{
			name:    "missing token",
			session: &Session{ctx: context.Background(), epControlPlane: "http://example.com", machineID: "machine-1"},
			req:     &DiagnosticRequest{ReportID: "diag_1", Type: diagnosticTypeNvidiaBugReport},
			reason:  "missing_session_token",
		},
		{
			name:    "missing machine id",
			session: &Session{ctx: context.Background(), epControlPlane: "http://example.com", token: "token"},
			req:     &DiagnosticRequest{ReportID: "diag_1", Type: diagnosticTypeNvidiaBugReport},
			reason:  "missing_machine_id",
		},
		{
			name: "missing process runner",
			session: &Session{
				ctx:                      context.Background(),
				epControlPlane:           "http://example.com",
				machineID:                "machine-1",
				token:                    "token",
				diagnosticExecutablePath: createExecutableFile(t),
			},
			req:    &DiagnosticRequest{ReportID: "diag_1", Type: diagnosticTypeNvidiaBugReport},
			reason: "missing_process_runner",
		},
		{
			name: "missing executable",
			session: &Session{
				ctx:                      context.Background(),
				epControlPlane:           "http://example.com",
				machineID:                "machine-1",
				token:                    "token",
				processRunner:            new(mockProcessRunner),
				diagnosticExecutablePath: filepath.Join(t.TempDir(), "missing"),
			},
			req:    &DiagnosticRequest{ReportID: "diag_1", Type: diagnosticTypeNvidiaBugReport},
			reason: "diagnostic_executable_unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &Response{}
			tt.session.processDiagnostic(Request{Diagnostic: tt.req}, response)
			require.NotNil(t, response.Diagnostic)
			assert.False(t, response.Diagnostic.Accepted)
			assert.Equal(t, tt.reason, response.Diagnostic.Reason)
		})
	}
}

func TestProcessDiagnosticAcceptsAndRunsInBackground(t *testing.T) {
	type upload struct {
		body            []byte
		auth            string
		sha             string
		machineID       string
		legacyMachineID string
		origin          string
	}
	uploads := make(chan upload, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/diagnostics/diag_1/report", r.URL.Path)
		uploaded, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploads <- upload{
			body:            uploaded,
			auth:            r.Header.Get("Authorization"),
			sha:             r.Header.Get(diagnosticSHA256Header),
			machineID:       r.Header.Get("X-GPUD-Machine-ID"),
			legacyMachineID: r.Header.Get("machine_id"),
			origin:          r.Header.Get("Origin"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := new(mockProcessRunner)
	runner.On("RunUntilCompletion", mock.Anything, mock.MatchedBy(func(script string) bool {
		return strings.Contains(script, "nvidia-bug-report.sh' --output-file") &&
			strings.Contains(script, "report.log.gz")
	})).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		assert.NotNil(t, ctx)
		script := args.String(1)
		reportPath := extractReportPathFromScript(t, script)
		require.NoError(t, os.WriteFile(reportPath, []byte("bug-report"), 0600))
	}).Return([]byte("created\n"), 0, nil)

	session := diagnosticTestSession(t, runner)
	session.epControlPlane = server.URL
	response := &Response{}
	session.processDiagnostic(Request{Diagnostic: &DiagnosticRequest{
		ReportID:       "diag_1",
		Type:           diagnosticTypeNvidiaBugReport,
		TimeoutSeconds: 1,
	}}, response)

	require.NotNil(t, response.Diagnostic)
	assert.True(t, response.Diagnostic.Accepted)

	var got upload
	select {
	case got = <-uploads:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for diagnostic upload")
	}
	assert.Equal(t, "Bearer test-token", got.auth)
	assert.Equal(t, "test-machine", got.machineID)
	assert.Equal(t, "test-machine", got.legacyMachineID)
	assert.Equal(t, "127.0.0.1", got.origin)
	expectedSHA := sha256.Sum256(got.body)
	assert.Equal(t, hex.EncodeToString(expectedSHA[:]), got.sha)
	reader, err := gzip.NewReader(bytes.NewReader(got.body))
	require.NoError(t, err)
	defer func() {
		_ = reader.Close()
	}()
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, []byte("bug-report"), decoded)
	runner.AssertExpectations(t)
}

func TestProcessDiagnosticRejectsConcurrentDiagnostic(t *testing.T) {
	session := diagnosticTestSession(t, new(mockProcessRunner))
	session.diagnosticRunning = true

	response := &Response{}
	session.processDiagnostic(Request{Diagnostic: &DiagnosticRequest{
		ReportID: "diag_1",
		Type:     diagnosticTypeNvidiaBugReport,
	}}, response)

	require.NotNil(t, response.Diagnostic)
	assert.False(t, response.Diagnostic.Accepted)
	assert.Equal(t, "diagnostic_already_running", response.Diagnostic.Reason)
}

func TestRunDiagnosticDoesNotUploadSyntheticSuccessWhenReportMissing(t *testing.T) {
	uploadc := make(chan struct{}, 1)
	failurec := make(chan diagnosticFailureRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnostics/diag_1/report":
			uploadc <- struct{}{}
		case "/api/v1/diagnostics/diag_1/failure":
			var req diagnosticFailureRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			failurec <- req
		default:
			t.Errorf("unexpected diagnostic request path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := new(mockProcessRunner)
	runner.On("RunUntilCompletion", mock.Anything, mock.Anything).Return([]byte("not found"), 127, assert.AnError)

	session := diagnosticTestSession(t, runner)
	session.epControlPlane = server.URL
	require.True(t, session.beginDiagnostic())
	session.runDiagnostic(DiagnosticRequest{ReportID: "diag_1", Type: diagnosticTypeNvidiaBugReport})

	select {
	case <-uploadc:
		t.Fatal("missing report should not upload synthetic success artifact")
	default:
	}
	select {
	case failure := <-failurec:
		assert.Equal(t, diagnosticFailureCommandFailed, failure.Reason)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for diagnostic failure notification")
	}
	runner.AssertExpectations(t)
	assert.False(t, session.diagnosticRunning)
}

func TestRunDiagnosticNotifiesFailureOnTimeout(t *testing.T) {
	type failure struct {
		req             diagnosticFailureRequest
		auth            string
		machineID       string
		legacyMachineID string
		origin          string
		contentType     string
	}
	failures := make(chan failure, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/diagnostics/diag_1/failure", r.URL.Path)
		var req diagnosticFailureRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		failures <- failure{
			req:             req,
			auth:            r.Header.Get("Authorization"),
			machineID:       r.Header.Get("X-GPUD-Machine-ID"),
			legacyMachineID: r.Header.Get("machine_id"),
			origin:          r.Header.Get("Origin"),
			contentType:     r.Header.Get("Content-Type"),
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := new(mockProcessRunner)
	runner.On("RunUntilCompletion", mock.Anything, mock.Anything).Return([]byte("timed out"), 124, context.DeadlineExceeded)

	session := diagnosticTestSession(t, runner)
	session.epControlPlane = server.URL
	require.True(t, session.beginDiagnostic())
	session.runDiagnostic(DiagnosticRequest{ReportID: "diag_1", Type: diagnosticTypeNvidiaBugReport, TimeoutSeconds: 1})

	var got failure
	select {
	case got = <-failures:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for diagnostic failure notification")
	}
	assert.Equal(t, diagnosticFailureTimeout, got.req.Reason)
	assert.Equal(t, "Bearer test-token", got.auth)
	assert.Equal(t, "test-machine", got.machineID)
	assert.Equal(t, "test-machine", got.legacyMachineID)
	assert.Equal(t, "127.0.0.1", got.origin)
	assert.Equal(t, diagnosticFailureContentType, got.contentType)
	runner.AssertExpectations(t)
	assert.False(t, session.diagnosticRunning)
}

func TestRunDiagnosticNotifiesFailureAfterUploadFailure(t *testing.T) {
	failures := make(chan diagnosticFailureRequest, 1)
	var uploadAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/diagnostics/diag_1/report":
			uploadAttempts.Add(1)
			http.Error(w, "bad upload", http.StatusForbidden)
		case "/api/v1/diagnostics/diag_1/failure":
			var req diagnosticFailureRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			failures <- req
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected diagnostic request path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	runner := new(mockProcessRunner)
	runner.On("RunUntilCompletion", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		reportPath := extractReportPathFromScript(t, args.String(1))
		require.NoError(t, os.WriteFile(reportPath, []byte("bug-report"), 0600))
	}).Return([]byte("created\n"), 0, nil)

	session := diagnosticTestSession(t, runner)
	session.epControlPlane = server.URL
	require.True(t, session.beginDiagnostic())
	session.runDiagnostic(DiagnosticRequest{ReportID: "diag_1", Type: diagnosticTypeNvidiaBugReport})

	select {
	case failure := <-failures:
		assert.Equal(t, diagnosticFailureUploadFailed, failure.Reason)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for diagnostic failure notification")
	}
	assert.Equal(t, int32(1), uploadAttempts.Load())
	runner.AssertExpectations(t)
	assert.False(t, session.diagnosticRunning)
}

func TestUploadDiagnosticReportRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad upload", http.StatusBadRequest)
	}))
	defer server.Close()

	session := diagnosticTestSession(t, nil)
	session.epControlPlane = server.URL
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.log.gz")
	require.NoError(t, os.WriteFile(reportPath, []byte("report"), 0600))

	err := session.uploadDiagnosticReport(DiagnosticRequest{ReportID: "diag_1"}, reportPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

func TestUploadDiagnosticReportWithRetryRetriesServerErrors(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			http.Error(w, "try again", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	session := diagnosticTestSession(t, nil)
	session.epControlPlane = server.URL
	session.timeSleepFunc = func(time.Duration) {}
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.log.gz")
	require.NoError(t, os.WriteFile(reportPath, []byte("report"), 0600))

	err := session.uploadDiagnosticReportWithRetry(DiagnosticRequest{ReportID: "diag_1"}, reportPath)
	require.NoError(t, err)
	assert.Equal(t, int32(2), attempts.Load())
}

func TestEnsureGzipReportCompressesPlainReport(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.log.gz")
	original := []byte("plain report")
	require.NoError(t, os.WriteFile(reportPath, original, 0600))

	require.NoError(t, ensureGzipReport(reportPath))
	raw, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer func() {
		_ = reader.Close()
	}()
	text, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, original, text)
}

func TestNvidiaBugReportScriptUsesFixedCommand(t *testing.T) {
	script := nvidiaBugReportScript(diagnosticNvidiaBugReportPath, "/tmp/work dir", "/tmp/work dir/report.log.gz")
	assert.Contains(t, script, shellQuote(diagnosticNvidiaBugReportPath)+" --output-file")
	assert.NotContains(t, script, "nvidia_bug_report")
	assert.Contains(t, script, shellQuote("/tmp/work dir"))
	assert.Contains(t, script, shellQuote("/tmp/work dir/report.log.gz"))
}

func diagnosticTestSession(t *testing.T, runner *mockProcessRunner) *Session {
	t.Helper()
	if runner == nil {
		runner = new(mockProcessRunner)
	}
	return &Session{
		ctx:                      context.Background(),
		epControlPlane:           "http://example.com",
		machineID:                "test-machine",
		token:                    "test-token",
		processRunner:            new(mockProcessRunner),
		diagnosticProcessRunner:  runner,
		diagnosticExecutablePath: createExecutableFile(t),
		auditLogger:              log.NewNopAuditLogger(),
		writer:                   make(chan Body, 1),
	}
}

func createExecutableFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nvidia-bug-report.sh")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0700))
	return path
}

func extractReportPathFromScript(t *testing.T, script string) string {
	t.Helper()
	const prefix = "report_path='"
	start := strings.Index(script, prefix)
	require.NotEqual(t, -1, start)
	start += len(prefix)
	end := strings.Index(script[start:], "'")
	require.NotEqual(t, -1, end)
	return script[start : start+end]
}
