package powersupply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
)

type Output struct {
	BatteryCapacity      uint64 `json:"battery_capacity"`
	BatteryCapacityFound bool   `json:"battery_capacity_found"`
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
	StateNamePowerSupply = "power_supply"

	StateKeyBatteryCapacity      = "battery_capacity"
	StateKeyBatteryCapacityFound = "battery_capacity_found"
)

func ParseStatePowerSupply(m map[string]string) (*Output, error) {
	o := &Output{}

	var err error
	o.BatteryCapacity, err = strconv.ParseUint(m[StateKeyBatteryCapacity], 10, 64)
	if err != nil {
		return nil, err
	}
	o.BatteryCapacityFound, err = strconv.ParseBool(m[StateKeyBatteryCapacityFound])
	if err != nil {
		return nil, err
	}

	return o, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	for _, state := range states {
		switch state.Name {
		case StateNamePowerSupply:
			return ParseStatePowerSupply(state.ExtraInfo)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, fmt.Errorf("no state found")
}

func (o *Output) States() ([]components.State, error) {
	state := components.State{
		Name:    StateNamePowerSupply,
		Healthy: true,
		Reason:  fmt.Sprintf("power supply check success, battery capacity: %d, battery capacity found: %v", o.BatteryCapacity, o.BatteryCapacityFound),
		ExtraInfo: map[string]string{
			StateKeyBatteryCapacity:      fmt.Sprintf("%d", o.BatteryCapacity),
			StateKeyBatteryCapacityFound: fmt.Sprintf("%v", o.BatteryCapacityFound),
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
		defaultPoller = query.New(Name, cfg.Query, Get)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func Get(ctx context.Context) (_ any, e error) {
	defer func() {
		if e != nil {
			components_metrics.SetGetFailed(Name)
		} else {
			components_metrics.SetGetSuccess(Name)
		}
	}()

	_, err := os.Stat(DefaultBatteryCapacityFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Output{
				BatteryCapacity:      0,
				BatteryCapacityFound: false,
			}, nil
		}
		return nil, err
	}
	batteryCapacity, err := getBatteryCapacityFile()
	if err != nil {
		return nil, err
	}
	return &Output{
		BatteryCapacity:      batteryCapacity,
		BatteryCapacityFound: true,
	}, nil
}

const DefaultBatteryCapacityFile = "/sys/class/power_supply/BAT0/capacity"

func getBatteryCapacityFile() (uint64, error) {
	capacity, err := os.ReadFile(DefaultBatteryCapacityFile)
	if err != nil {
		return 0, err
	}
	capacityInt, err := strconv.ParseUint(string(capacity), 10, 64)
	if err != nil {
		return 0, err
	}
	return capacityInt, nil
}
