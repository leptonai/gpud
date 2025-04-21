package customplugins

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// CustomPluginRegisteree is an optional interface that can be implemented by components
// to allow them to be registered as custom plugins.
type CustomPluginRegisteree interface {
	// IsCustomPlugin returns true if the component is a custom plugin.
	IsCustomPlugin() bool
	// Spec returns the custom plugin spec.
	Spec() Spec
}

// Specs is a list of plugin specs.
type Specs []Spec

// Spec is a plugin spec and configuration.
// Each spec represents a single state or event, in the external-plugin component.
type Spec struct {
	// PluginName describes the plugin.
	// It is used for generating the component name.
	PluginName string `json:"plugin_name"`

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

// Plugin represents a plugin spec.
type Plugin struct {
	// Steps is a sequence of steps to run for this plugin.
	// The steps are executed in order.
	// If a step fails, the execution stops and the error is returned.
	Steps []Step `json:"steps,omitempty"`
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
