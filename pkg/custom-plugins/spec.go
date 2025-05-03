package customplugins

import (
	"errors"
	"fmt"
	"os"
	"strings"
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

	return pluginSpecs.ExpandedValidate()
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

// ExpandedValidate expands the component list and validates all specs.
func (pluginSpecs Specs) ExpandedValidate() (Specs, error) {
	pluginSpecs, err := pluginSpecs.ExpandComponentList()
	if err != nil {
		return nil, err
	}

	for i := range pluginSpecs {
		if err := pluginSpecs[i].Validate(); err != nil {
			return nil, err
		}
	}

	return pluginSpecs, nil
}

// ExpandComponentList expands the component list into multiple components.
func (pluginSpecs Specs) ExpandComponentList() (Specs, error) {
	expandedSpecs := make([]Spec, 0)

	for _, spec := range pluginSpecs {
		if spec.Type != SpecTypeComponentList {
			expandedSpecs = append(expandedSpecs, spec)
			continue
		}

		if spec.ComponentListFile != "" {
			// Fail if component list is not empty
			if len(spec.ComponentList) > 0 {
				return nil, fmt.Errorf("component list must be empty when using component_list_file")
			}
			// Read the file
			content, err := os.ReadFile(spec.ComponentListFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read component list file %s: %w", spec.ComponentListFile, err)
			}

			// Split into lines and trim whitespace
			lines := strings.Split(string(content), "\n")
			spec.ComponentList = make([]string, 0, len(lines))
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				spec.ComponentList = append(spec.ComponentList, line)
			}

			if len(spec.ComponentList) == 0 {
				return nil, fmt.Errorf("component list file %s is empty", spec.ComponentListFile)
			}
		}

		if len(spec.ComponentList) == 0 {
			return nil, fmt.Errorf("component list is empty")
		}

		// Validate each component name in the list
		for _, component := range spec.ComponentList {
			if component == "" {
				return nil, fmt.Errorf("component name cannot be empty in component list")
			}
			// Split on ':' to get name#run_mode and parameter
			parts := strings.SplitN(component, ":", 2)
			// Split on '#' to get name and run_mode
			nameParts := strings.SplitN(parts[0], "#", 2)
			if nameParts[0] == "" {
				return nil, fmt.Errorf("component name cannot be empty in component list")
			}
		}

		if spec.ComponentName() == "" {
			return nil, ErrComponentNameRequired
		}

		if spec.HealthStatePlugin == nil {
			return nil, ErrMissingStatePlugin
		}

		// Create a new plugin for each component in the list
		for _, component := range spec.ComponentList {
			// Split on ':' to get name#run_mode and parameter
			parts := strings.SplitN(component, ":", 2)
			// Split on '#' to get name and run_mode
			nameParts := strings.SplitN(parts[0], "#", 2)
			name := nameParts[0]
			runMode := spec.RunMode // Default to parent's run_mode
			if len(nameParts) > 1 {
				runMode = nameParts[1]
			}
			param := ""
			if len(parts) > 1 {
				param = parts[1]
			}

			// Create a new plugin with substituted parameters
			expandedPlugin := Spec{
				PluginName: name,
				Type:       SpecTypeComponent,
				RunMode:    runMode,
				HealthStatePlugin: &Plugin{
					Steps:  make([]Step, len(spec.HealthStatePlugin.Steps)),
					Parser: spec.HealthStatePlugin.Parser,
				},
				Timeout:  spec.Timeout,
				Interval: spec.Interval,
			}

			// Copy and substitute each step
			for i, step := range spec.HealthStatePlugin.Steps {
				if step.RunBashScript != nil {
					// Substitute parameters in the script
					script := step.RunBashScript.Script
					script = strings.ReplaceAll(script, "${NAME}", name)
					script = strings.ReplaceAll(script, "${PAR}", param)
					script = strings.ReplaceAll(script, "${PAR1}", param)

					expandedPlugin.HealthStatePlugin.Steps[i] = Step{
						Name: step.Name,
						RunBashScript: &RunBashScript{
							ContentType: step.RunBashScript.ContentType,
							Script:      script,
						},
					}
				}
			}

			expandedSpecs = append(expandedSpecs, expandedPlugin)
		}
	}

	return expandedSpecs, nil
}

// Validate validates the plugin spec.
func (spec *Spec) Validate() error {
	switch spec.Type {
	// Allow only init and component types, not component list which should have been expanded by this point.
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
