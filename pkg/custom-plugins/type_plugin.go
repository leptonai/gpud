package customplugins

import (
	"context"
	"fmt"

	"github.com/leptonai/gpud/pkg/process"
)

// Validate validates all the plugin steps.
func (p *Plugin) Validate() error {
	for _, b := range p.Steps {
		if err := b.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// executeAllSteps runs all the plugin steps, and returns the output and its exit code.
func (p *Plugin) executeAllSteps(ctx context.Context) ([]byte, int32, error) {
	// one shared runner for all the steps in this plugin
	// run them in sequence, one by one
	// this is to avoid running multiple commands in parallel
	processRunner := process.NewExclusiveRunner()

	var err error
	output := make([]byte, 0)
	exitCode := int32(0)

	for _, b := range p.Steps {
		switch {
		case b.RunBashScript != nil:
			var out []byte
			out, exitCode, err = b.RunBashScript.executeBash(ctx, processRunner)
			if len(out) > 0 {
				output = append(output, out...)
			}

			// if a step fails, do not continue to the next step
			// return the error and exit code
			if exitCode != 0 || err != nil {
				return output, exitCode, err
			}

		default:
			return nil, 0, fmt.Errorf("unsupported plugin step: %T", b)
		}
	}

	return output, exitCode, nil
}
