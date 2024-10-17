package latency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/pkg/derp"
)

type Output struct {
	// DERPLatencies is the list of DERP latencies from global DERP servers (e.g., tailscale).
	DERPLatencies []derp.Latency `json:"derp_latencies"`
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

func (o *Output) States(cfg Config) ([]components.State, error) {
	unhealthyReasons := []string{}
	if cfg.GlobalMillisecondThreshold > 0 {
		for _, latency := range o.DERPLatencies {
			if latency.LatencyMilliseconds > cfg.GlobalMillisecondThreshold {
				unhealthyReasons = append(unhealthyReasons, fmt.Sprintf("latency to %s edge derp server (%s) exceeded threshold of %dms", latency.RegionName, latency.Latency, cfg.GlobalMillisecondThreshold))
			}
		}
	}

	healthy := true
	if cfg.GlobalMillisecondThreshold > 0 && len(unhealthyReasons) > 0 {
		if len(unhealthyReasons) == len(o.DERPLatencies) {
			healthy = false
		}
	}

	reason := "no issue"
	if len(unhealthyReasons) > 0 {
		reason = strings.Join(unhealthyReasons, "; ")
	}

	b, _ := o.JSON()
	state := components.State{
		Name:    StateNameLatency,
		Healthy: healthy,
		Reason:  reason,
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

		// "ctx" here is the root level, create one with shorter timeouts
		// to not block on this checks
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		var err error
		o.DERPLatencies, err = derp.MeasureLatencies(cctx)
		if err != nil {
			return nil, err
		}

		return o, nil
	}
}
