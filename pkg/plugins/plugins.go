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
	Name string `json:"name"`
	// GPUd canonical component name based on user-provided plugin name
	componentName string

	// StateJob represents the jobs to run for /states API.
	StateJob *Job `json:"state_job,omitempty"`
	// EventJob represents the jobs to run for /events API.
	EventJob *Job `json:"event_job,omitempty"`

	// EventScript is the script to run to g
	// set to true to allow non-zero exit code on the script
	// useful for dry runs
	DryRun bool `json:"dry_run"`

	// Timeout is the timeout for the script execution.
	Timeout metav1.Duration `json:"timeout"`

	// Interval is the interval for the script execution.
	// If zero, it runs only once.
	Interval metav1.Duration `json:"interval"`
}

type Job struct {
	// Script is the script to run for this job.
	// Assumed to be base64 encoded.
	Script string `json:"script"`
	// base64 decoded state script
	scriptDecoded string
}

func (job *Job) decode() error {
	if job != nil && job.Script != "" {
		decoded, err := base64.StdEncoding.DecodeString(job.Script)
		if err != nil {
			return err
		}
		job.scriptDecoded = string(decoded)
	}

	return nil
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

	if p.StateJob == nil || p.StateJob.Script == "" {
		return ErrStateScriptRequired
	}
	if err := p.StateJob.decode(); err != nil {
		return err
	}

	if p.EventJob != nil {
		if err := p.EventJob.decode(); err != nil {
			return err
		}
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
		if err := plugins[i].Validate(); err != nil {
			return nil, err
		}
	}

	return plugins, nil
}

// TODO
func (p *Plugin) CheckOnce(ctx context.Context) error {
	return nil
}
