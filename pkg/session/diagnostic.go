package session

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

const (
	diagnosticTypeNvidiaBugReport  = "nvidia_bug_report"
	diagnosticNvidiaBugReportPath  = "/usr/bin/nvidia-bug-report.sh"
	defaultDiagnosticTimeout       = 10 * time.Minute
	defaultDiagnosticUploadTimeout = 10 * time.Minute
	diagnosticReportContentType    = "application/gzip"
	diagnosticFailureContentType   = "application/json"
	diagnosticSHA256Header         = "X-GPUD-Diagnostic-SHA256"

	diagnosticFailureTimeout       = "diagnostic_timeout"
	diagnosticFailureCommandFailed = "diagnostic_command_failed"
	diagnosticFailureReportMissing = "diagnostic_report_missing"
	diagnosticFailureGzipFailed    = "diagnostic_gzip_failed"
	diagnosticFailureUploadFailed  = "diagnostic_upload_failed"
)

type diagnosticFailureRequest struct {
	Reason string `json:"reason,omitempty"`
}

// processDiagnostic accepts a fixed diagnostic request and starts the
// collection/upload workflow in the background. It only returns a short
// accepted/rejected response over the session stream.
func (s *Session) processDiagnostic(payload Request, response *Response) {
	req := payload.Diagnostic
	if req == nil {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "missing_diagnostic_request"}
		return
	}
	if req.ReportID == "" {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "missing_report_id"}
		return
	}
	if req.Type != diagnosticTypeNvidiaBugReport {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "unsupported_diagnostic_type"}
		return
	}
	if s.epControlPlane == "" {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "missing_control_plane_endpoint"}
		return
	}
	if s.machineID == "" {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "missing_machine_id"}
		return
	}
	if s.getToken() == "" {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "missing_session_token"}
		return
	}
	if s.diagnosticRunner() == nil {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "missing_process_runner"}
		return
	}
	if err := executableReady(s.diagnosticExecutable()); err != nil {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "diagnostic_executable_unavailable"}
		log.Logger.Warnw("diagnostic executable unavailable", "path", s.diagnosticExecutable(), "error", err)
		return
	}
	if !s.beginDiagnostic() {
		response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: false, Reason: "diagnostic_already_running"}
		return
	}

	reqCopy := *req
	response.Diagnostic = &DiagnosticAcceptedResponse{Accepted: true}
	go s.runDiagnostic(reqCopy)
}

func (s *Session) beginDiagnostic() bool {
	s.diagnosticMu.Lock()
	defer s.diagnosticMu.Unlock()
	if s.diagnosticRunning {
		return false
	}
	s.diagnosticRunning = true
	return true
}

func (s *Session) endDiagnostic() {
	s.diagnosticMu.Lock()
	defer s.diagnosticMu.Unlock()
	s.diagnosticRunning = false
}

func (s *Session) runDiagnostic(req DiagnosticRequest) {
	defer s.endDiagnostic()

	workDir, err := os.MkdirTemp("", "gpud-diagnostic-*")
	if err != nil {
		log.Logger.Errorw("failed to create diagnostic temp dir", "reportID", req.ReportID, "error", err)
		return
	}
	defer func() {
		if err := os.RemoveAll(workDir); err != nil {
			log.Logger.Warnw("failed to remove diagnostic temp dir", "reportID", req.ReportID, "error", err)
		}
	}()

	reportPath := filepath.Join(workDir, "report.log.gz")
	output, exitCode, runErr, failureReason := s.runNvidiaBugReport(req, workDir, reportPath)
	if runErr != nil {
		log.Logger.Warnw(
			"nvidia diagnostic command failed",
			"reportID", req.ReportID,
			"exitCode", exitCode,
			"error", runErr,
		)
	}

	if _, err := os.Stat(reportPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Logger.Warnw("failed to stat diagnostic report", "reportID", req.ReportID, "error", err)
		}
		log.Logger.Errorw("diagnostic report was not produced", "reportID", req.ReportID, "exitCode", exitCode, "error", runErr, "outputBytes", len(output))
		if failureReason == "" {
			failureReason = diagnosticFailureReportMissing
		}
		if err := s.notifyDiagnosticFailureWithRetry(req, failureReason); err != nil {
			log.Logger.Errorw("failed to notify diagnostic failure", "reportID", req.ReportID, "reason", failureReason, "error", err)
		}
		return
	}

	if err := ensureGzipReport(reportPath); err != nil {
		log.Logger.Errorw("failed to gzip diagnostic report", "reportID", req.ReportID, "error", err)
		if notifyErr := s.notifyDiagnosticFailureWithRetry(req, diagnosticFailureGzipFailed); notifyErr != nil {
			log.Logger.Errorw("failed to notify diagnostic failure", "reportID", req.ReportID, "reason", diagnosticFailureGzipFailed, "error", notifyErr)
		}
		return
	}

	if err := s.uploadDiagnosticReportWithRetry(req, reportPath); err != nil {
		log.Logger.Errorw("failed to upload diagnostic report", "reportID", req.ReportID, "error", err)
		if notifyErr := s.notifyDiagnosticFailureWithRetry(req, diagnosticFailureUploadFailed); notifyErr != nil {
			log.Logger.Errorw("failed to notify diagnostic failure", "reportID", req.ReportID, "reason", diagnosticFailureUploadFailed, "error", notifyErr)
		}
	}
}

func (s *Session) runNvidiaBugReport(req DiagnosticRequest, workDir, reportPath string) ([]byte, int32, error, string) {
	timeout := defaultDiagnosticTimeout
	if req.TimeoutSeconds > 0 {
		timeout = time.Duration(req.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(sessionContext(s.ctx), timeout)
	defer cancel()

	output, exitCode, err := s.diagnosticRunner().RunUntilCompletion(ctx, nvidiaBugReportScript(s.diagnosticExecutable(), workDir, reportPath))
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return output, exitCode, err, diagnosticFailureTimeout
	case err != nil:
		return output, exitCode, err, diagnosticFailureCommandFailed
	default:
		return output, exitCode, nil, ""
	}
}

func nvidiaBugReportScript(executablePath, workDir, reportPath string) string {
	quotedExecutablePath := shellQuote(executablePath)
	quotedWorkDir := shellQuote(workDir)
	quotedReportPath := shellQuote(reportPath)
	return fmt.Sprintf(`set -o pipefail
set -o nounset

cd %s
report_path=%s

%s --output-file "${report_path}"
exit_code=$?

if [ ! -f "${report_path}" ]; then
  for candidate in "${report_path}.gz" "nvidia-bug-report.log.gz"; do
    if [ -f "${candidate}" ]; then
      mv "${candidate}" "${report_path}"
      break
    fi
  done
fi

if [ ! -f "${report_path}" ]; then
  echo "nvidia-bug-report.sh did not create ${report_path}" >&2
  exit 1
fi

exit "${exit_code}"
`, quotedWorkDir, quotedReportPath, quotedExecutablePath)
}

func ensureGzipReport(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	var header [2]byte
	n, readErr := file.Read(header[:])
	_ = file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if n == 2 && header[0] == 0x1f && header[1] == 0x8b {
		return nil
	}

	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = in.Close()
	}()

	tempPath := path + ".gzip.tmp"
	out, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	_, copyErr := io.Copy(zw, in)
	closeErr := zw.Close()
	fileCloseErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}
	if fileCloseErr != nil {
		_ = os.Remove(tempPath)
		return fileCloseErr
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

type diagnosticUploadHTTPError struct {
	statusCode int
	body       string
}

func (e *diagnosticUploadHTTPError) Error() string {
	return fmt.Sprintf("diagnostic upload failed: status %d: %s", e.statusCode, e.body)
}

func (e *diagnosticUploadHTTPError) retryable() bool {
	return e.statusCode == http.StatusTooManyRequests || e.statusCode >= http.StatusInternalServerError
}

func (s *Session) uploadDiagnosticReport(req DiagnosticRequest, reportPath string) error {
	sha, err := sha256File(reportPath)
	if err != nil {
		return fmt.Errorf("failed to calculate diagnostic report sha256: %w", err)
	}
	origin, err := controlPlaneOrigin(s.epControlPlane)
	if err != nil {
		return err
	}

	file, err := os.Open(reportPath)
	if err != nil {
		return fmt.Errorf("failed to open diagnostic report: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	ctx, cancel := context.WithTimeout(sessionContext(s.ctx), defaultDiagnosticUploadTimeout)
	defer cancel()

	uploadURL := strings.TrimRight(s.epControlPlane, "/") + "/api/v1/diagnostics/" + url.PathEscape(req.ReportID) + "/report"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, file)
	if err != nil {
		return fmt.Errorf("failed to create diagnostic upload request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+s.getToken())
	httpReq.Header.Set("X-GPUD-Machine-ID", s.machineID)
	httpReq.Header.Set("machine_id", s.machineID)
	httpReq.Header.Set("Origin", origin)
	httpReq.Header.Set(httputil.RequestHeaderContentType, diagnosticReportContentType)
	httpReq.Header.Set(diagnosticSHA256Header, sha)
	if st, err := file.Stat(); err == nil {
		httpReq.ContentLength = st.Size()
	}

	resp, err := createHTTPClient(nil).Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to upload diagnostic report: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &diagnosticUploadHTTPError{statusCode: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	return nil
}

func (s *Session) uploadDiagnosticReportWithRetry(req DiagnosticRequest, reportPath string) error {
	var lastErr error
	backoffs := []time.Duration{0, time.Second, 5 * time.Second}
	for attempt, backoff := range backoffs {
		if backoff > 0 {
			s.sleep(backoff)
		}
		err := s.uploadDiagnosticReport(req, reportPath)
		if err == nil {
			return nil
		}
		lastErr = err

		var httpErr *diagnosticUploadHTTPError
		if errors.As(err, &httpErr) && !httpErr.retryable() {
			return err
		}
		log.Logger.Warnw("diagnostic upload attempt failed", "reportID", req.ReportID, "attempt", attempt+1, "error", err)
	}
	return lastErr
}

func (s *Session) notifyDiagnosticFailure(req DiagnosticRequest, reason string) error {
	origin, err := controlPlaneOrigin(s.epControlPlane)
	if err != nil {
		return err
	}
	body, err := json.Marshal(diagnosticFailureRequest{Reason: reason})
	if err != nil {
		return fmt.Errorf("failed to marshal diagnostic failure request: %w", err)
	}

	ctx, cancel := context.WithTimeout(sessionContext(s.ctx), defaultDiagnosticUploadTimeout)
	defer cancel()

	failureURL := strings.TrimRight(s.epControlPlane, "/") + "/api/v1/diagnostics/" + url.PathEscape(req.ReportID) + "/failure"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, failureURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create diagnostic failure request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+s.getToken())
	httpReq.Header.Set("X-GPUD-Machine-ID", s.machineID)
	httpReq.Header.Set("machine_id", s.machineID)
	httpReq.Header.Set("Origin", origin)
	httpReq.Header.Set(httputil.RequestHeaderContentType, diagnosticFailureContentType)

	resp, err := createHTTPClient(nil).Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to notify diagnostic failure: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &diagnosticUploadHTTPError{statusCode: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	return nil
}

func (s *Session) notifyDiagnosticFailureWithRetry(req DiagnosticRequest, reason string) error {
	var lastErr error
	backoffs := []time.Duration{0, time.Second, 5 * time.Second}
	for attempt, backoff := range backoffs {
		if backoff > 0 {
			s.sleep(backoff)
		}
		err := s.notifyDiagnosticFailure(req, reason)
		if err == nil {
			return nil
		}
		lastErr = err

		var httpErr *diagnosticUploadHTTPError
		if errors.As(err, &httpErr) && !httpErr.retryable() {
			return err
		}
		log.Logger.Warnw("diagnostic failure notification attempt failed", "reportID", req.ReportID, "reason", reason, "attempt", attempt+1, "error", err)
	}
	return lastErr
}

func (s *Session) sleep(d time.Duration) {
	if s.timeSleepFunc != nil {
		s.timeSleepFunc(d)
		return
	}
	time.Sleep(d)
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func (s *Session) diagnosticExecutable() string {
	if s.diagnosticExecutablePath != "" {
		return s.diagnosticExecutablePath
	}
	return diagnosticNvidiaBugReportPath
}

func (s *Session) diagnosticRunner() process.Runner {
	if s.diagnosticProcessRunner != nil {
		return s.diagnosticProcessRunner
	}
	return s.processRunner
}

func executableReady(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	if st.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}

func sessionContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
