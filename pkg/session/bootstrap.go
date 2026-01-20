package session

import (
	"context"
	"encoding/base64"
	"time"
)

// processBootstrap handles bootstrap script execution.
//
// IMPORTANT: Bootstrap scripts commonly end with backgrounded commands like:
//
//	sleep 10 && systemctl restart gpud &
//
// This pattern allows the script to exit immediately while scheduling a delayed
// restart of gpud. The processRunner.RunUntilCompletion() method is configured
// with a grace period (WithWaitForDetach) to ensure these backgrounded commands
// are not killed when the script exits and Close() is called.
//
// Without the grace period, the backgrounded "sleep 10 && systemctl restart gpud"
// would be killed immediately when the parent bash script exits, preventing the
// scheduled restart from occurring.
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

	// RunUntilCompletion uses WithWaitForDetach(2*time.Minute) internally to handle
	// backgrounded commands like "sleep 10 && systemctl restart gpud &".
	// See runner_exclusive.go for the implementation.
	cctx, cancel := context.WithTimeout(ctx, timeout)
	output, exitCode, err := s.processRunner.RunUntilCompletion(cctx, string(script))
	cancel()
	response.Bootstrap = &BootstrapResponse{
		Output:   string(output),
		ExitCode: exitCode,
	}
	if err != nil {
		response.Error = err.Error()
	}
}
