package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	tailscale_id "github.com/leptonai/gpud/components/tailscale/id"
	"github.com/leptonai/gpud/poller"

	"sigs.k8s.io/yaml"
)

type Output struct {
	Version VersionInfo `json:"version"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNameTailscaleVersion = "tailscale_version"

	StateKeyTailscaleVersionData           = "data"
	StateKeyTailscaleVersionEncoding       = "encoding"
	StateValueTailscaleVersionEncodingJSON = "json"
)

func ParseStateTailscale(m map[string]string) (*Output, error) {
	o := &Output{}

	ver := VersionInfo{}
	data := m[StateKeyTailscaleVersionData]
	if err := json.Unmarshal([]byte(data), &ver); err != nil {
		return nil, err
	}
	o.Version = ver

	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameTailscaleVersion:
			return ParseStateTailscale(state.ExtraInfo)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, fmt.Errorf("no state found")
}

// Returns the output evaluation reason and its healthy-ness.
func (o *Output) Evaluate() (string, bool, error) {
	yb, err := yaml.Marshal(o.Version)
	if err != nil {
		return "", false, err
	}
	return fmt.Sprintf("version:\n\n%s", string(yb)), true, nil
}

func (o *Output) States() ([]components.State, error) {
	outputReasons, healthy, err := o.Evaluate()
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(o.Version)
	state := components.State{
		Name:    StateNameTailscaleVersion,
		Healthy: healthy,
		Reason:  outputReasons,
		ExtraInfo: map[string]string{
			StateKeyTailscaleVersionData:     string(b),
			StateKeyTailscaleVersionEncoding: StateValueTailscaleVersionEncodingJSON,
		},
	}
	return []components.State{state}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     poller.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = poller.New(
			tailscale_id.Name,
			cfg.PollerConfig,
			Get,
			nil,
		)
	})
}

func getDefaultPoller() poller.Poller {
	return defaultPoller
}

func Get(ctx context.Context) (_ any, e error) {
	defer func() {
		if e != nil {
			components_metrics.SetGetFailed(tailscale_id.Name)
		} else {
			components_metrics.SetGetSuccess(tailscale_id.Name)
		}
	}()

	ver, err := CheckVersion()
	if err != nil {
		return nil, err
	}
	if ver == nil {
		return nil, errors.New("tailscale version is nil")
	}
	return &Output{Version: *ver}, nil
}
