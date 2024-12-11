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
	components_metrics "github.com/leptonai/gpud/components/metrics"
	network_latency_id "github.com/leptonai/gpud/components/network/latency/id"
	"github.com/leptonai/gpud/components/network/latency/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/pkg/latency"
	latency_edge "github.com/leptonai/gpud/pkg/latency/edge"
)

type Output struct {
	// EgressLatencies is the list of egress latencies to global edge servers.
	EgressLatencies latency.Latencies `json:"egress_latencies"`
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
		for _, latency := range o.EgressLatencies {
			if latency.LatencyMilliseconds > cfg.GlobalMillisecondThreshold {
				unhealthyReasons = append(unhealthyReasons, fmt.Sprintf("latency to %s edge derp server (%s) exceeded threshold of %dms", latency.RegionName, latency.Latency, cfg.GlobalMillisecondThreshold))
			}
		}
	}

	healthy := true
	if cfg.GlobalMillisecondThreshold > 0 && len(unhealthyReasons) > 0 {
		if len(unhealthyReasons) == len(o.EgressLatencies) {
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
		defaultPoller = query.New(network_latency_id.Name, cfg.Query, createGetFunc(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func createGetFunc(cfg Config) query.GetFunc {
	timeout := time.Duration(2*cfg.GlobalMillisecondThreshold) * time.Millisecond
	if timeout < 15*time.Second {
		timeout = 15 * time.Second
	}

	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(network_latency_id.Name)
			} else {
				components_metrics.SetGetSuccess(network_latency_id.Name)
			}
		}()

		now := time.Now().UTC()
		nowUTC := float64(now.Unix())
		metrics.SetLastUpdateUnixSeconds(nowUTC)

		o := &Output{}

		// "ctx" here is the root level, create one with shorter timeouts
		// to not block on this checks
		cctx, ccancel := context.WithTimeout(ctx, timeout)
		defer ccancel()

		var err error
		o.EgressLatencies, err = latency_edge.Measure(cctx)
		if err != nil {
			return nil, err
		}

		for _, latency := range o.EgressLatencies {
			if err := metrics.SetEdgeInMilliseconds(
				cctx,
				fmt.Sprintf("%s (%s)", latency.RegionName, latency.Provider),
				float64(latency.LatencyMilliseconds),
				now,
			); err != nil {
				return nil, err
			}
		}

		return o, nil
	}
}
