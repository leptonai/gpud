package customplugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"sigs.k8s.io/yaml"
)

// Validate validates all the plugin specs.
func (specs Specs) Validate() error {
	all := make(map[string]struct{})
	for i := range specs {
		if err := specs[i].Validate(); err != nil {
			return fmt.Errorf("failed to validate plugin spec: %w (plugin: %q)", err, specs[i].ComponentName())
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

var (
	ErrInvalidPluginType     = errors.New("invalid plugin type")
	ErrComponentNameRequired = errors.New("component name is required")
	ErrStepNameRequired      = errors.New("step name is required")
	ErrMissingPluginStep     = errors.New("plugin step cannot be empty")
	ErrMissingStatePlugin    = errors.New("state plugin is required")
	ErrScriptRequired        = errors.New("script is required")
	ErrIntervalTooShort      = errors.New("interval is too short")
)

const (
	MaxPluginNameLength = 128
	DefaultTimeout      = time.Minute
)

// Validate validates the plugin spec.
func (spec *Spec) Validate() error {
	switch spec.Type {
	case SpecTypeInit, SpecTypeComponent:
	default:
		return ErrInvalidPluginType
	}

	if len(spec.PluginName) > MaxPluginNameLength {
		return fmt.Errorf("plugin name is too long: %s", spec.PluginName)
	}
	if spec.ComponentName() == "" {
		return ErrComponentNameRequired
	}

	if spec.HealthStatePlugin == nil {
		return ErrMissingStatePlugin
	}
	if err := spec.HealthStatePlugin.Validate(); err != nil {
		return err
	}

	if spec.Timeout.Duration == 0 {
		spec.Timeout.Duration = DefaultTimeout
	}

	if spec.Interval.Duration > 0 && spec.Interval.Duration < time.Minute {
		return ErrIntervalTooShort
	}

	return nil
}

// ComponentName returns the component name for the plugin spec.
func (spec *Spec) ComponentName() string {
	return ConvertToComponentName(spec.PluginName)
}

// RunStatePlugin runs the state plugin and returns the output and its exit code.
func (spec *Spec) RunStatePlugin(ctx context.Context) ([]byte, int32, error) {
	if spec.HealthStatePlugin == nil {
		return nil, 0, ErrMissingStatePlugin
	}
	if err := spec.HealthStatePlugin.Validate(); err != nil {
		return nil, 0, err
	}
	if spec.DryRun {
		return nil, 0, nil
	}

	cctx, cancel := context.WithTimeout(ctx, spec.Timeout.Duration)
	defer cancel()
	return spec.HealthStatePlugin.executeAllSteps(cctx)
}
