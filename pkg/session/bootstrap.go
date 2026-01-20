package session

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/leptonai/gpud/pkg/process"
)

// processBootstrap handles bootstrap script execution.
//
// Uses WithAllowDetachedProcess(true) to allow backgrounded commands (using "&")
// to continue running after the script exits. This is critical for bootstrap
// scripts that use patterns like:
//
//	sleep 10 && systemctl restart gpud &
//
// Without WithAllowDetachedProcess(true):
// - The "&" does not take effect - backgrounded processes are killed on Close()
// - gpud_init.sh waits 10s and restarts instead of returning instantly
// - Control plane thinks bootstrap failed (script didn't return quickly)
// - Control plane sends gpud_init.sh again
// - This causes gpud to restart repeatedly in a loop
func (s *Session) processBootstrap(ctx context.Context, payload Request, response *Response) {
	if payload.Bootstrap == nil {
		return
	}

	script, err := base64.StdEncoding.DecodeString(payload.Bootstrap.ScriptBase64)
	if err != nil {
		response.Error = err.Error()
		return
	}

	timeout := time.Duration(payload.Bootstrap.TimeoutInSeconds) * time.Second
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	// WithAllowDetachedProcess(true) allows "sleep 10 && systemctl restart gpud &" to detach.
	// Without this, the backgrounded command is killed, causing the script to block for 10s
	// instead of returning immediately, making control plane think bootstrap failed.
	output, exitCode, err := s.processRunner.RunUntilCompletion(cctx, string(script), process.WithAllowDetachedProcess(true))
	cancel()
	response.Bootstrap = &BootstrapResponse{
		Output:   string(output),
		ExitCode: exitCode,
	}
	if err != nil {
		response.Error = err.Error()
	}
}
