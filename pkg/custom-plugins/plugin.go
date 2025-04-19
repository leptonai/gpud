package customplugins

import (
	"context"
	"fmt"

	"github.com/leptonai/gpud/pkg/process"
)

// Plugin represents a plugin spec.
type Plugin struct {
	// Steps is a sequence of steps to run for this plugin.
	// The steps are executed in order.
	// If a step fails, the execution stops and the error is returned.
	Steps []PluginStep `json:"steps,omitempty"`
}

// Validate validates all the plugin steps.
func (p Plugin) Validate() error {
	for _, b := range p.Steps {
		if err := b.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// executeAllSteps runs all the plugin steps, and returns the output and its exit code.
func (p Plugin) executeAllSteps(ctx context.Context) ([]byte, int32, error) {
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

// PluginStep represents a step in a plugin.
type PluginStep struct {
	// Name is the name of the step.
	Name string `json:"name,omitempty"`

	// RunBashScript is the bash script to run for this step.
	RunBashScript *RunBashScript `json:"run_bash_script,omitempty"`

	// TODO
	// we may support other ways to run plugins in the future
	// e.g., container image
}

// Validate validates the plugin step.
func (p PluginStep) Validate() error {
	if p.Name == "" {
		return ErrStepNameRequired
	}

	switch {
	case p.RunBashScript != nil:
		return p.RunBashScript.Validate()

	default:
		return ErrMissingPluginStep
	}
}
