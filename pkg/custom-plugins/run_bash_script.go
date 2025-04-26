package customplugins

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// Validate validates the run bash script.
func (b *RunBashScript) Validate() error {
	if b.Script == "" {
		return ErrScriptRequired
	}

	if _, err := b.decode(); err != nil {
		return err
	}

	return nil
}

// decode decodes the script based on the content type.
func (b *RunBashScript) decode() (string, error) {
	if b.Script == "" {
		return "", nil
	}

	switch b.ContentType {
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(b.Script)
		if err != nil {
			return "", err
		}
		return string(decoded), nil

	case "plaintext", "text/plain":
		return b.Script, nil

	default:
		return "", fmt.Errorf("unsupported content type: %s", b.ContentType)
	}
}

// executeBash runs the specified bash script and returns the output and its exit code.
func (b *RunBashScript) executeBash(ctx context.Context, processRunner process.Runner) ([]byte, int32, error) {
	decoded, err := b.decode()
	if err != nil {
		return nil, 0, err
	}

	execOut, exitCode, err := processRunner.RunUntilCompletion(ctx, decoded)
	if err != nil {
		log.Logger.Errorw("failed to run bash script", "output", string(execOut), "exitCode", exitCode, "error", err)
	} else {
		log.Logger.Debugw("bash script output", "output", string(execOut), "exitCode", exitCode)
	}

	return execOut, exitCode, err
}
