package latency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/network/latency/derpmap"
	"github.com/leptonai/gpud/components/query"
)

type Output struct {
	RegionLatencies []RegionLatency `json:"region_latency"`
}

type RegionLatency struct {
	// RegionID/RegionCode list is available at https://login.tailscale.com/derpmap/default
	RegionID         int           `json:"region_id"`         // RegionID is the DERP region ID
	RegionCode       string        `json:"region_code"`       // RegionCode is the three-letter code for the region
	RegionName       string        `json:"region_name"`       // RegionName is the human-readable name of the region (e.g. "Chicago")
	Latency          time.Duration `json:"latency"`           // Latency is the round-trip time to the region
	LatencyHumanized string        `json:"latency_humanized"` // LatencyHumanized is the human-readable version of the latency, in milliseconds
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
	StateNameLatency = "network-latency"

	StateKeyLatencyData         = "data"
	StateKeyLatencyEncoding     = "encoding"
	StateKeyLatencyEncodingJSON = "json"
)

func ParseStateLatency(m map[string]string) (*Output, error) {
	data := m[StateKeyLatencyData]
	return ParseOutputJSON([]byte(data))

}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNameLatency:
			o, err := ParseStateLatency(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			return o, nil

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, errors.New("no latency state found")
}

func (o *Output) States() ([]components.State, error) {
	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameLatency,
		Healthy: true,
		Reason:  "n/a",
		ExtraInfo: map[string]string{
			StateKeyLatencyData:     string(b),
			StateKeyLatencyEncoding: StateKeyLatencyEncodingJSON,
		},
	}
	return []components.State{state}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(Name, cfg.Query, createGetFunc(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func createGetFunc(cfg Config) query.GetFunc {
	return func(ctx context.Context) (any, error) {
		o := &Output{}
		// get code -> region mapping
		// TODO: support region code, region name, and region ID in config file
		codeMap := derpmap.SavedDERPMap.GetRegionCodeMapping()
		for _, regionCode := range cfg.RegionCodes {
			region, ok := codeMap[regionCode]
			if !ok {
				return nil, fmt.Errorf("region code %s not found", regionCode)
			}
			rl := RegionLatency{
				RegionID:   region.RegionID,
				RegionCode: regionCode,
				RegionName: region.RegionName,
			}

			latency, err := getRegionLatency(ctx, region.RegionID)
			if err != nil {
				return nil, err
			}
			rl.Latency = latency
			rl.LatencyHumanized = fmt.Sprintf("%v", latency)
			o.RegionLatencies = append(o.RegionLatencies, rl)
		}
		return o, nil
	}
}

func getRegionLatency(ctx context.Context, regionID int) (time.Duration, error) {
	// TODO: implmenet getRegionLatency - see https://github.com/tailscale/tailscale/blob/main/net/netcheck/netcheck.go#L950
	return time.Second, nil
}
