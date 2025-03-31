package systemd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	systemd_id "github.com/leptonai/gpud/components/systemd/id"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/query"
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

const (
	StateNameSystemd       = "systemd"
	StateKeySystemdVersion = "version"

	StateNameUnit               = "unit"
	StateKeyUnitName            = "name"
	StateKeyUnitActive          = "active"
	StateKeyUnitUptimeSeconds   = "uptime_seconds"
	StateKeyUnitUptimeHumanized = "uptime_humanized"
)

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
		defaultPoller = query.New(
			systemd_id.Name,
			cfg.Query,
			CreateGet(cfg),
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		ver, _, err := systemd.CheckVersion()
		if err != nil {
			return nil, err
		}
		o := &Output{SystemdVersion: ver}

		// create a new dbus connection on demand
		// as the long-running connection may fail in the middle
		// e.g.,
		//  read unix @->/run/dbus/system_bus_socket: use of closed network connection)
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		dbusConn, err := systemd.NewDbusConn(ctx)
		if err != nil {
			return nil, err
		}
		defer dbusConn.Close()

		for _, unit := range cfg.Units {
			active := false

			cctx, ccancel := context.WithTimeout(ctx, 15*time.Second)
			active, err = dbusConn.IsActive(cctx, unit)
			ccancel()

			if err != nil {
				log.Logger.Warnw("failed to check active status", "unit", unit, "error", err)
				active, err = systemd.IsActive(unit)
				if err != nil {
					return nil, fmt.Errorf("failed to check active status for unit %q: %w", unit, err)
				}
			}

			uptimeSeconds := int64(0)
			uptimeDescription := "n/a"

			uptime, err := systemd.GetUptime(unit)
			if err != nil {
				log.Logger.Errorw("failed to get uptime for unit", "unit", unit, "error", err)
			}

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
