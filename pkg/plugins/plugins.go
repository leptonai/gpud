package plugins

import (
	"context"
	"encoding/base64"
	"errors"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// Plugins is a list of plugins.
type Plugins []Plugin

// Plugin is a plugin.
type Plugin struct {
	// Name is the name of the plugin.
	// Does not allow whitespace characters.
	Name          string `json:"name"`
	componentName string

	// StateScript is the script to run to get the state of the plugin.
	// Assumed to be base64 encoded.
	StateScript        string `json:"state_script"`
	stateScriptDecoded string

	// EventScript is the script to run to get the event of the plugin.
	// Assumed to be base64 encoded.
	EventScript        string `json:"event_script,omitempty"`
	eventScriptDecoded string

	// set to true to allow non-zero exit code on the script
	// useful for dry runs
	DryRun bool `json:"dry_run"`

	// Timeout is the timeout for the script execution.
	Timeout metav1.Duration `json:"timeout"`

	// Interval is the interval for the script execution.
	Interval metav1.Duration `json:"interval"`
}

var (
	ErrComponentNameRequired = errors.New("component name is required")
	ErrStateScriptRequired   = errors.New("state script is required")
	ErrTimeoutRequired       = errors.New("timeout is required")
)

func (p *Plugin) Validate() error {
	if p.ComponentName() == "" {
		return ErrComponentNameRequired
	}

	if err := p.decode(); err != nil {
		return err
	}

	if p.StateScript == "" {
		return ErrStateScriptRequired
	}

	if p.Timeout.Duration == 0 {
		return ErrTimeoutRequired
	}

	return nil
}

func (p *Plugin) ComponentName() string {
	if p.componentName == "" {
		p.componentName = ConvertToComponentName(p.Name)
	}
	return p.componentName
}

func (p *Plugin) decode() error {
	if p.StateScript != "" {
		decoded, err := base64.StdEncoding.DecodeString(p.StateScript)
		if err != nil {
			return err
		}
		p.stateScriptDecoded = string(decoded)
	}

	if p.EventScript != "" {
		decoded, err := base64.StdEncoding.DecodeString(p.EventScript)
		if err != nil {
			return err
		}
		p.eventScriptDecoded = string(decoded)
	}

	return nil
}

// Load loads the plugins from the given path.
func Load(path string) (Plugins, error) {
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var plugins Plugins
	if err := yaml.Unmarshal(yamlFile, &plugins); err != nil {
		return nil, err
	}

	for i := range plugins {
		if err := plugins[i].decode(); err != nil {
			return nil, err
		}
	}

	return plugins, nil
}

// TODO
func (p *Plugin) Run(_ context.Context) error {
	return nil
}
