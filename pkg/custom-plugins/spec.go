package customplugins

import (
	"context"
	"errors"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Specs is a list of plugin specs.
type Specs []Spec

// Validate validates all the plugin specs.
func (specs Specs) Validate() error {
	all := make(map[string]struct{})
	for i := range specs {
		if err := specs[i].Validate(); err != nil {
			return err
		}

		if _, ok := all[specs[i].ComponentName()]; ok {
			return fmt.Errorf("duplicate component name: %s", specs[i].ComponentName())
		}
		all[specs[i].ComponentName()] = struct{}{}
	}

	return nil
}

// LoadSpecs loads the plugin specs from the given path.
func LoadSpecs(path string) (Specs, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pluginSpecs Specs
	if err := yaml.Unmarshal(yamlFile, &pluginSpecs); err != nil {
		return nil, err
	}

	for i := range pluginSpecs {
		if err := pluginSpecs[i].Validate(); err != nil {
			return nil, err
		}
	}

	return pluginSpecs, nil
}

// Spec is a plugin spec and configuration.
// Each spec represents a single state or event, in the external-plugin component.
type Spec struct {
	// Name describes the plugin.
	// It is used for generating the component name.
	Name string `json:"name"`

	// StatePlugin represents the jobs to run for /states API.
	StatePlugin *Plugin `json:"state_plugin,omitempty"`

	// DryRun is set to true to allow non-zero exit code on the script
	// useful for dry runs.
	DryRun bool `json:"dry_run"`

	// Timeout is the timeout for the script execution.
	Timeout metav1.Duration `json:"timeout"`

	// Interval is the interval for the script execution.
	// If zero, it runs only once.
	Interval metav1.Duration `json:"interval"`
}

var (
	ErrComponentNameRequired = errors.New("component name is required")
	ErrTimeoutRequired       = errors.New("timeout is required")
	ErrStepNameRequired      = errors.New("step name is required")
	ErrMissingPluginStep     = errors.New("plugin step cannot be empty")
	ErrMissingStatePlugin    = errors.New("state plugin is required")
	ErrStateScriptRequired   = errors.New("state script is required")
)

// Validate validates the plugin spec.
func (spec *Spec) Validate() error {
	if spec.ComponentName() == "" {
		return ErrComponentNameRequired
	}

	if spec.StatePlugin == nil {
		return ErrMissingStatePlugin
	}

	if spec.Timeout.Duration == 0 {
		return ErrTimeoutRequired
	}

	return nil
}

// ComponentName returns the component name for the plugin spec.
func (spec *Spec) ComponentName() string {
	return ConvertToComponentName(spec.Name)
}

// RunStatePlugin runs the state plugin and returns the output and its exit code.
func (spec *Spec) RunStatePlugin(ctx context.Context) ([]byte, int32, error) {
	if spec.StatePlugin == nil {
		return nil, 0, ErrMissingStatePlugin
	}
	if err := spec.StatePlugin.Validate(); err != nil {
		return nil, 0, err
	}
	if spec.DryRun {
		return nil, 0, nil
	}

	cctx, cancel := context.WithTimeout(ctx, spec.Timeout.Duration)
	defer cancel()
	return spec.StatePlugin.executeAllSteps(cctx)
}
