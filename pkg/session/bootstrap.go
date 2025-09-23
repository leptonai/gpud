package session

import (
	"context"
	"encoding/base64"
	"time"
)

// processBootstrap handles bootstrap script execution
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
