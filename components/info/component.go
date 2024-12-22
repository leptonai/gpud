// Package info provides static information about the host (e.g., labels, IDs).
package info

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/components"
	info_id "github.com/leptonai/gpud/components/info/id"
	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/manager"
	"github.com/leptonai/gpud/version"
)

func New(annotations map[string]string, db *sql.DB) components.Component {
	return &component{
		annotations: annotations,
		db:          db,
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	annotations map[string]string
	db          *sql.DB
}

func (c *component) Name() string { return info_id.Name }

const (
	StateNameDaemon = "daemon"

	StateKeyDaemonVersion = "daemon_version"
	StateKeyMacAddress    = "mac_address"
	StateKeyPackages      = "packages"

	StateNameGPUD                    = "gpud"
	StateKeyGPUdMachineID            = "gpud_machine_id"
	StateKeyGPUdLoggedIn             = "gpud_logged_in"
	StateKeyGPUdLoginTimeUnixSeconds = "gpud_login_time_unix_seconds"
	StateKeyGPUdLoginTimeHumanized   = "gpud_login_time_humanized"

	StateNameAnnotations = "annotations"
)

func (c *component) States(ctx context.Context) ([]components.State, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	mac := ""
	for _, iface := range interfaces {
		macAddress := iface.HardwareAddr.String()
		if macAddress != "" {
			mac = macAddress
			break
		}
	}

	var managedPackages string
	if manager.GlobalController != nil {
		packageStatus, err := manager.GlobalController.Status(ctx)
		if err != nil {
			return nil, err
		}
		rawPayload, _ := json.Marshal(&packageStatus)
		managedPackages = string(rawPayload)
	}

	states := []components.State{
		{
			Name:    StateNameDaemon,
			Healthy: true,
			Reason:  fmt.Sprintf("daemon version: %s, mac address: %s", version.Version, mac),
			ExtraInfo: map[string]string{
				StateKeyDaemonVersion: version.Version,
				StateKeyMacAddress:    mac,
				StateKeyPackages:      managedPackages,
			},
		},
		{
			Name:      StateNameAnnotations,
			Healthy:   true,
			Reason:    fmt.Sprintf("annotations: %v", c.annotations),
			ExtraInfo: c.annotations,
		},
	}

	if c.db != nil {
		machineID, err := state.ReadMachineID(ctx, c.db)
		if err != nil {
			return nil, err
		}

		loginInfo, err := state.GetLoginInfo(ctx, c.db, machineID)
		if err != nil {
			return nil, err
		}

		loggedIn := loginInfo != nil
		var (
			loginTimeUnixSeconds string
			loginTimeHumanized   string
		)
		if loggedIn {
			loginTime := loginInfo.LoginTime
			loginTimeUnixSeconds = fmt.Sprintf("%d", loginTime.Unix())
			loginTimeHumanized = humanize.RelTime(loginTime, time.Now().UTC(), "ago", "from now")
		}

		states = append(states, components.State{
			Name:    StateNameGPUD,
			Healthy: true,
			Reason:  fmt.Sprintf("machine ID: %s", machineID),
			ExtraInfo: map[string]string{
				StateKeyGPUdMachineID:            machineID,
				StateKeyGPUdLoggedIn:             strconv.FormatBool(loggedIn),
				StateKeyGPUdLoginTimeUnixSeconds: loginTimeUnixSeconds,
				StateKeyGPUdLoginTimeHumanized:   loginTimeHumanized,
			},
		})
	}

	return states, nil
}

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	return nil
}

var _ components.SettableComponent = (*component)(nil)

func (c *component) SetStates(ctx context.Context, states ...components.State) error {
	for _, s := range states {
		if s.Name == "annotations" {
			for k, v := range s.ExtraInfo {
				c.annotations[k] = v
			}
		}
	}
	return nil
}

func (c *component) SetEvents(ctx context.Context, events ...components.Event) error {
	panic("not implemented")
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}
