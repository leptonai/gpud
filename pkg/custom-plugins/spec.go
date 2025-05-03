package customplugins

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"
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
	case SpecTypeInit, SpecTypeComponent, SpecTypeComponentList:
	default:
		return ErrInvalidPluginType
	}

	if len(spec.PluginName) > MaxPluginNameLength {
		return fmt.Errorf("plugin name is too long: %s", spec.PluginName)
	}

	if spec.Type == SpecTypeComponentList {
		// Handle component_list_file if present
		if spec.ComponentListFile != "" {
			// Read the file
			content, err := os.ReadFile(spec.ComponentListFile)
			if err != nil {
				return fmt.Errorf("failed to read component list file %s: %w", spec.ComponentListFile, err)
			}

			// Split into lines and trim whitespace
			lines := strings.Split(string(content), "\n")
			spec.ComponentList = make([]string, 0, len(lines))
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					spec.ComponentList = append(spec.ComponentList, line)
				}
			}

			if len(spec.ComponentList) == 0 {
				return fmt.Errorf("component list file %s is empty", spec.ComponentListFile)
			}
		}

		if len(spec.ComponentList) == 0 {
			return fmt.Errorf("component list cannot be empty for type %s", SpecTypeComponentList)
		}
		// Validate each component name in the list
		for _, component := range spec.ComponentList {
			if component == "" {
				return fmt.Errorf("component name cannot be empty in component list")
			}
			// Split on ':' to get name#run_mode and parameter
			parts := strings.SplitN(component, ":", 2)
			// Split on '#' to get name and run_mode
			nameParts := strings.SplitN(parts[0], "#", 2)
			if nameParts[0] == "" {
				return fmt.Errorf("component name cannot be empty in component list")
			}
		}
	} else if spec.ComponentName() == "" {
		return ErrComponentNameRequired
	}

	if spec.HealthStatePlugin == nil {
		return ErrMissingStatePlugin
	}

	// If this is a component list, expand it into multiple components
	if spec.Type == SpecTypeComponentList {
		// Create a new plugin for each component in the list
		expandedPlugins := make([]*Plugin, 0, len(spec.ComponentList))
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
			expandedPlugin := &Plugin{
				Steps:  make([]Step, len(spec.HealthStatePlugin.Steps)),
				Parser: spec.HealthStatePlugin.Parser,
			}

			// Copy and substitute each step
			for i, step := range spec.HealthStatePlugin.Steps {
				if step.RunBashScript != nil {
					// Substitute parameters in the script
					script := step.RunBashScript.Script
					script = strings.ReplaceAll(script, "${NAME}", name)
					script = strings.ReplaceAll(script, "${PAR}", param)
					script = strings.ReplaceAll(script, "${PAR1}", param)

					expandedPlugin.Steps[i] = Step{
						Name: step.Name,
						RunBashScript: &RunBashScript{
							ContentType: step.RunBashScript.ContentType,
							Script:      script,
						},
					}
				}
			}

			// Create a new spec for this component
			componentSpec := &Spec{
				PluginName:        name,
				Type:             SpecTypeComponent,
				RunMode:          runMode,
				HealthStatePlugin: expandedPlugin,
				Timeout:           spec.Timeout,
				Interval:          spec.Interval,
			}

			expandedPlugins = append(expandedPlugins, expandedPlugin)
		}

		// Replace the original plugin with the first expanded plugin
		spec.HealthStatePlugin = expandedPlugins[0]
		// The rest of the expanded plugins will be handled by the caller
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
