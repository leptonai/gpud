package customplugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
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

func (pluginSpecs Specs) PrintValidateResults(wr io.Writer, checkMark string, warningSign string) {
	table := tablewriter.NewWriter(wr)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)
	table.SetHeader([]string{"Component", "Type", "Run Mode", "Timeout", "Interval", "Valid"})
	for _, spec := range pluginSpecs {
		verr := spec.Validate()
		s := checkMark + " valid"
		if verr != nil {
			s = warningSign + " invalid"
		}
		table.Append([]string{spec.ComponentName(), spec.PluginType, spec.RunMode, spec.Timeout.Duration.String(), spec.Interval.Duration.String(), s})
	}
	table.Render()
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

func (a Specs) Equal(b Specs) bool {
	if len(a) != len(b) {
		return false
	}

	aD, _ := json.Marshal(a)
	bD, _ := json.Marshal(b)
	return bytes.Equal(aD, bD)
}

// SaveSpecs loads the plugin specs from the given path.
// It returns true, if the specs are created or updated.
// It returns false, if the specs are not updated,
// because the specs are the same as the existing ones.
func SaveSpecs(path string, newSpecs Specs) (bool, error) {
	_, err := os.Stat(path)
	alreadyExists := err == nil

	if !alreadyExists && !os.IsNotExist(err) {
		return false, err
	}

	if alreadyExists {
		b, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		var existingSpecs Specs
		if err := yaml.Unmarshal(b, &existingSpecs); err != nil {
			return false, err
		}
		if existingSpecs.Equal(newSpecs) {
			return false, nil
		}
		// already exists, but not the same as the new specs
		// still need to overwrite the file
	}

	b, err := yaml.Marshal(newSpecs)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		return false, err
	}
	return true, nil
}

var (
	ErrInvalidPluginType        = errors.New("invalid plugin type")
	ErrComponentNameRequired    = errors.New("component name is required")
	ErrStepNameRequired         = errors.New("step name is required")
	ErrMissingPluginStep        = errors.New("plugin step cannot be empty")
	ErrMissingStatePlugin       = errors.New("state plugin is required")
	ErrScriptRequired           = errors.New("script is required")
	ErrIntervalTooShort         = errors.New("interval is too short")
	ErrComponentListNotExpanded = errors.New("component list must be expanded before validation")
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

// parseComponentListEntry parses a component list entry.
// The entry can be in the following formats:
// - "name"
// - "name:param"
// - "name#run_mode[tag1,tag2]"
// - "name#run_mode[tag1,tag2]:param"
func parseComponentListEntry(entry string) (name, param, runMode string, tags []string, err error) {
	// First split by ':' to separate name and param
	parts := strings.SplitN(entry, ":", 2)
	namePart := parts[0]
	if len(parts) > 1 {
		param = parts[1]
	}

	// Then split by '#' to separate name and run mode
	nameParts := strings.SplitN(namePart, "#", 2)
	name = nameParts[0]
	if name == "" {
		return "", "", "", nil, fmt.Errorf("component name cannot be empty in component list")
	}

	if len(nameParts) > 1 {
		runModePart := nameParts[1]
		// Check if run mode contains tags
		hasOpenBracket := strings.Contains(runModePart, "[")
		hasCloseBracket := strings.Contains(runModePart, "]")
		if hasOpenBracket != hasCloseBracket {
			return "", "", "", nil, fmt.Errorf("invalid tag format in component list entry: %s", entry)
		}
		if hasOpenBracket && hasCloseBracket {
			// Extract run mode and tags
			runModeEnd := strings.Index(runModePart, "[")
			runMode = strings.TrimSpace(runModePart[:runModeEnd])
			tagsStr := runModePart[runModeEnd+1 : len(runModePart)-1]
			if tagsStr == "" {
				tags = []string{}
			} else {
				tags = strings.Split(tagsStr, ",")
				// Trim spaces from tags and filter out empty tags
				validTags := make([]string, 0, len(tags))
				for _, tag := range tags {
					if trimmed := strings.TrimSpace(tag); trimmed != "" {
						validTags = append(validTags, trimmed)
					}
				}
				tags = validTags
			}
		} else {
			runMode = strings.TrimSpace(runModePart)
		}
	}

	return name, param, runMode, tags, nil
}

// ExpandComponentList expands the component list into multiple components.
func (pluginSpecs Specs) ExpandComponentList() (Specs, error) {
	expandedSpecs := make([]Spec, 0)

	for _, spec := range pluginSpecs {
		if spec.PluginType != SpecTypeComponentList {
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

		if spec.ComponentName() == "" {
			return nil, ErrComponentNameRequired
		}

		if spec.HealthStatePlugin == nil {
			return nil, ErrMissingStatePlugin
		}

		// Create a new plugin for each component in the list
		for _, component := range spec.ComponentList {

			name, param, runMode, tags, err := parseComponentListEntry(component)
			if err != nil {
				return nil, err
			}

			// Use parent's run mode if not specified in the entry
			if runMode == "" {
				runMode = spec.RunMode
			}

			if tags == nil {
				tags = spec.Tags
			}

			// Create a new plugin with substituted parameters
			expandedPlugin := Spec{
				PluginName: name,
				PluginType: SpecTypeComponent,
				RunMode:    runMode,
				Tags:       tags,
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
	// for compatibility with the old spec format
	// which uses "type" instead of "plugin_type"
	if spec.DeprecatedType != "" && spec.PluginType == "" {
		spec.PluginType = spec.DeprecatedType
	}

	switch spec.PluginType {
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

	// Validate component list
	if spec.PluginType == SpecTypeComponentList {
		return ErrComponentListNotExpanded
	}

	return nil
}

// ComponentName returns the component name for the plugin spec.
func (spec *Spec) ComponentName() string {
	return ConvertToComponentName(spec.PluginName)
}
