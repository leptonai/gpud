package systemd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
	systemd_id "github.com/leptonai/gpud/components/systemd/id"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/systemd"

	"github.com/dustin/go-humanize"
)

type Output struct {
	SystemdVersion string `json:"systemd_version"`
	Units          []Unit `json:"units"`
}

type Unit struct {
	Name            string `json:"name"`
	Active          bool   `json:"active"`
	UptimeSeconds   int64  `json:"uptime_seconds"`
	UptimeHumanized string `json:"uptime_humanized"`
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
	StateNameSystemd       = "systemd"
	StateKeySystemdVersion = "version"

	StateNameUnit               = "unit"
	StateKeyUnitName            = "name"
	StateKeyUnitActive          = "active"
	StateKeyUnitUptimeSeconds   = "uptime_seconds"
	StateKeyUnitUptimeHumanized = "uptime_humanized"
)

func ParseStateSystemd(m map[string]string) (*Output, error) {
	o := &Output{}
	o.SystemdVersion = m[StateKeySystemdVersion]
	return o, nil
}

func ParseStateUnit(m map[string]string) (Unit, error) {
	u := Unit{}
	u.Name = m[StateKeyUnitName]

	b, err := strconv.ParseBool(m[StateKeyUnitActive])
	if err != nil {
		return Unit{}, err
	}
	u.Active = b

	v, err := strconv.ParseInt(m[StateKeyUnitUptimeSeconds], 10, 64)
	if err != nil {
		return Unit{}, err
	}
	u.UptimeSeconds = v
	u.UptimeHumanized = m[StateKeyUnitUptimeHumanized]

	return u, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	o := &Output{}
	for _, state := range states {
		switch state.Name {
		case StateNameSystemd:
			o.SystemdVersion = state.ExtraInfo[StateKeySystemdVersion]

		case StateNameUnit:
			u, err := ParseStateUnit(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Units = append(o.Units, u)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return nil, fmt.Errorf("no state found")
}

func (o *Output) States() ([]components.State, error) {
	cs := []components.State{
		{
			Name:    StateNameSystemd,
			Healthy: true,
			Reason:  fmt.Sprintf("version: %s", o.SystemdVersion),
			ExtraInfo: map[string]string{
				StateKeySystemdVersion: o.SystemdVersion,
			},
		},
	}
	for _, unit := range o.Units {
		cs = append(cs, components.State{
			Name:    StateNameUnit,
			Healthy: unit.Active,
			Reason:  fmt.Sprintf("name: %s, active: %v, uptime: %s", unit.Name, unit.Active, unit.UptimeHumanized),
			ExtraInfo: map[string]string{
				StateKeyUnitName:            unit.Name,
				StateKeyUnitActive:          strconv.FormatBool(unit.Active),
				StateKeyUnitUptimeSeconds:   fmt.Sprintf("%d", unit.UptimeSeconds),
				StateKeyUnitUptimeHumanized: unit.UptimeHumanized,
			},
		})
	}
	return cs, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(systemd_id.Name, cfg.Query, CreateGet(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(systemd_id.Name)
			} else {
				components_metrics.SetGetSuccess(systemd_id.Name)
			}
		}()

		ver, _, err := systemd.CheckVersion()
		if err != nil {
			return nil, err
		}
		o := &Output{SystemdVersion: ver}

		for _, unit := range cfg.Units {
			uptime, err := systemd.GetUptime(unit)
			if err != nil {
				return nil, fmt.Errorf("failed to get uptime for unit %q: %w", unit, err)
			}

			active := false

			defaultConn := GetDefaultDbusConn()
			if defaultConn != nil {
				active, err = defaultConn.IsActive(ctx, unit)
			}
			if defaultConn == nil || err != nil {
				active, err = systemd.IsActive(unit)
				if err != nil {
					return nil, fmt.Errorf("failed to check active status for unit %q: %w", unit, err)
				}
			}

			uptimeSeconds := int64(0)
			uptimeDescription := "n/a"
			if uptime != nil {
				uptimeSeconds = int64(uptime.Seconds())

				now := time.Now().UTC()
				uptimeDescription = humanize.RelTime(now.Add(-*uptime), now, "ago", "from now")
			}
			o.Units = append(o.Units, Unit{
				Name:            unit,
				Active:          active,
				UptimeSeconds:   uptimeSeconds,
				UptimeHumanized: uptimeDescription,
			})
		}

		return o, nil
	}
}

var (
	defaultDbusConnOnce sync.Once

	defaultDbusConnMu     sync.Mutex
	defaultDbusConn       *systemd.DbusConn
	defaultDbusConnCancel context.CancelFunc
)

func ConnectDbus() error {
	var err error
	defaultDbusConnOnce.Do(func() {
		for i := 0; i < 10; i++ {
			var ctx context.Context
			ctx, defaultDbusConnCancel = context.WithCancel(context.Background())
			defaultDbusConn, err = systemd.NewDbusConn(ctx)
			if err != nil {
				defaultDbusConnCancel()
				log.Logger.Debugw("failed to connect to dbus", "error", err)
				time.Sleep(time.Duration(i) * time.Second)
				continue
			}
			log.Logger.Debugw("successfully connect to dbus")
			return
		}
	})
	return err
}

func GetDefaultDbusConn() *systemd.DbusConn {
	defaultDbusConnMu.Lock()
	defer defaultDbusConnMu.Unlock()
	return defaultDbusConn
}

func CloseDefaultDbusConn() {
	defaultDbusConnMu.Lock()
	defer defaultDbusConnMu.Unlock()

	if defaultDbusConn != nil {
		defaultDbusConn.Close()
	}
	if defaultDbusConnCancel != nil {
		defaultDbusConnCancel()
	}
}
