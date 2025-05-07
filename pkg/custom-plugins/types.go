package customplugins

import (
	"fmt"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CustomPluginRegisteree is an optional interface that can be implemented by components
// to allow them to be registered as custom plugins.
type CustomPluginRegisteree interface {
	// IsCustomPlugin returns true if the component is a custom plugin.
	IsCustomPlugin() bool
	// Spec returns the custom plugin spec.
	Spec() Spec
}

const (
	// SpecTypeInit is the type of the plugin that is used to initialize at the server start.
	// Meant to be run only once.
	SpecTypeInit = "init"
	// SpecTypeComponent is the type of the plugin that is used to run as a component.
	// Meant to be run periodically.
	SpecTypeComponent = "component"
	// SpecTypeComponentList is the type of the plugin that is used to run as multiple components.
	// Each item in the component list becomes a separate component.
	SpecTypeComponentList = "component_list"
)

// Specs is a list of plugin specs.
type Specs []Spec

// Spec is a plugin spec and configuration.
// Each spec represents a single state or event, in the external-plugin component.
type Spec struct {
	// PluginName describes the plugin.
	// It is used for generating the component name.
	PluginName string `json:"plugin_name"`

	// Type defines the plugin type.
	// Possible values: "init", "component".
	Type string `json:"type"`

	// ComponentList is a list of component names for SpecTypeComponentList.
	// Each item can be a simple name or "name:param" format.
	// For component list, tags can be specified in the format "name#run_mode[tag1,tag2]:param"
	ComponentList []string `json:"component_list,omitempty"`

	// ComponentListFile is a path to a file containing component names for SpecTypeComponentList.
	// Each line can be a simple name or "name:param" format.
	// For component list file, tags can be specified in the format "name#run_mode[tag1,tag2]:param"
	ComponentListFile string `json:"component_list_file,omitempty"`

	// RunMode defines the run mode of the plugin.
	// Possible values: "auto", "manual".
	//
	// RunMode is set to "auto" to run the plugin periodically, with the specified interval.
	//
	// RunMode is set to "manual" to run the plugin only when explicitly triggered.
	// The manual mode plugin is only registered but not run periodically.
	// - GPUd does not run this even once.
	// - GPUd does not run this periodically.
	//
	// This "auto" mode is only applicable to "component" type plugins.
	// This "auto" mode is not applicable to "init" type plugins.
	//
	// The "init" type plugins are always run only once.
	// This "manual" mode is only applicable to "component" type plugins.
	// This "manual" mode is not applicable to "init" type plugins.
	RunMode string `json:"run_mode"`

	// Tags is a list of tags associated with this component.
	// Tags can be used to group and trigger components together.
	// For component list type, tags can also be specified in the run mode format.
	Tags []string `json:"tags,omitempty"`

	// HealthStatePlugin defines the plugin instructions
	// to evaluate the health state of this plugin,
	// which is translated into an GPUd /states API response.
	HealthStatePlugin *Plugin `json:"health_state_plugin,omitempty"`

	// Timeout is the timeout for the script execution.
	// If zero, it uses the default timeout (1-minute).
	Timeout metav1.Duration `json:"timeout"`

	// Interval is the interval for the script execution.
	// For init plugin that only runs once at the server start,
	// this value is ignored.
	// Similarly, if set to zero, it runs only once.
	Interval metav1.Duration `json:"interval"`
}

// Plugin represents a plugin spec.
type Plugin struct {
	// Steps is a sequence of steps to run for this plugin.
	// Multiple steps are executed in order.
	// If a step fails, the execution stops and the error is returned.
	// Which means, the final success requires all steps to succeed.
	Steps []Step `json:"steps,omitempty"`

	// Parser is the parser for the plugin output.
	// If not set, the default prefix parser is used.
	Parser *PluginOutputParseConfig `json:"parser,omitempty"`
}

// Step represents a step in a plugin.
type Step struct {
	// Name is the name of the step.
	Name string `json:"name,omitempty"`

	// RunBashScript is the bash script to run for this step.
	RunBashScript *RunBashScript `json:"run_bash_script,omitempty"`

	// TODO
	// we may support other ways to run plugins in the future
	// e.g., container image
}

// RunBashScript represents the bash script runtime.
type RunBashScript struct {
	// ContentType is the content encode type of the script.
	// Possible values: "plaintext", "base64".
	ContentType string `json:"content_type"`

	// Script is the script to run for this job.
	// Assumed to be base64 encoded.
	Script string `json:"script"`
}

// PluginOutputParseConfig configures the parser for the plugin output.
type PluginOutputParseConfig struct {
	// JSONPaths is a list of JSON paths to the output fields.
	// Each entry has a FieldName (the output field name you want to assign e.g. "name")
	// and a QueryPath (the JSON path you want to extract with e.g. "$.name").
	JSONPaths []JSONPath `json:"json_paths,omitempty"`
}

// JSONPath represents a JSON path to the output fields.
type JSONPath struct {
	// Query defines the JSONPath query path to extract with.
	// ref. https://pkg.go.dev/github.com/PaesslerAG/jsonpath#section-readme
	// ref. https://en.wikipedia.org/wiki/JSONPath
	// ref. https://goessner.net/articles/JsonPath/
	Query string `json:"query"`
	// Field defines the field name to use in the extra_info data
	// for this JSON path query output.
	Field string `json:"field"`

	// Expect defines the expected field "value" match rule.
	//
	// It not set, the field value is not checked,
	// which means "missing field" for this query does not
	// make the health state to be "Unhealthy".
	//
	// If set, the field value must be matched for this rule.
	// In such case, the "missing field" or "mismatch" make
	// the health state to be "Unhealthy".
	Expect *MatchRule `json:"expect,omitempty"`

	// SuggestedActions maps from the suggested action name,
	// to the match rule for the field value.
	//
	// If the field value matches the rule,
	// the health state reports the corresponding
	// suggested action (the key of the matching rule).
	SuggestedActions map[string]MatchRule `json:"suggested_actions,omitempty"`
}

// MatchRule represents an expected output match rule
// for the plugin output.
type MatchRule struct {
	// Regex is the regex to match the output.
	Regex *string `json:"regex,omitempty"`
}

// doesMatch checks if the input matches the match rule.
// It returns true if the input matches the match rule, otherwise false.
// It returns an error if the match rule is invalid.
func (rule *MatchRule) doesMatch(input string) (bool, error) {
	if rule == nil {
		// no rule then it matches
		return true, nil
	}

	if rule.Regex != nil {
		rule := *rule.Regex

		re, err := regexp.Compile(rule)
		if err != nil {
			return false, fmt.Errorf("failed to compile regex %q: %w", rule, err)
		}
		return re.MatchString(input), nil
	}

	return true, nil
}

func (rule *MatchRule) describeRule() string {
	if rule == nil {
		return ""
	}

	if rule.Regex != nil {
		return *rule.Regex
	}

	return ""
}
