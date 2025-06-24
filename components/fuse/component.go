// Package fuse monitors the FUSE (Filesystem in Userspace).
package fuse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/fuse"
	"github.com/leptonai/gpud/pkg/log"
)

// Name is the name of the component.
const Name = "fuse"

const (
	// DefaultCongestedPercentAgainstThreshold is the percentage of the FUSE connections waiting
	// at which we consider the system to be congested.
	DefaultCongestedPercentAgainstThreshold = float64(90)
	// DefaultMaxBackgroundPercentAgainstThreshold is the percentage of the FUSE connections waiting
	// at which we consider the system to be congested.
	DefaultMaxBackgroundPercentAgainstThreshold = float64(80)
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	// congestedPercentAgainstThreshold is the percentage of the FUSE connections waiting
	// at which we consider the system to be congested.
	congestedPercentAgainstThreshold float64
	// maxBackgroundPercentAgainstThreshold is the percentage of the FUSE connections waiting
	// at which we consider the system to be congested.
	maxBackgroundPercentAgainstThreshold float64

	listConnectionsFunc func() (fuse.ConnectionInfos, error)

	eventBucket eventstore.Bucket

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		congestedPercentAgainstThreshold:     DefaultCongestedPercentAgainstThreshold,
		maxBackgroundPercentAgainstThreshold: DefaultMaxBackgroundPercentAgainstThreshold,

		listConnectionsFunc: fuse.ListConnections,
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		Name,
	}
}

func (c *component) IsSupported() bool {
	return true
}

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket == nil {
		return nil, nil
	}
	evs, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	return evs.Events(), nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking fuse")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	infos, err := c.listConnectionsFunc()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error listing fuse connections"
		log.Logger.Warnw(cr.reason, "error", cr.err)
		return cr
	}

	now := time.Now().UTC()

	foundDev := make(map[string]fuse.ConnectionInfo)
	for _, info := range infos {
		// to dedup fuse connection stats by device name
		if _, ok := foundDev[info.DeviceName]; ok {
			continue
		}
		foundDev[info.DeviceName] = info

		metricConnsCongestedPct.With(prometheus.Labels{"device_name": info.DeviceName}).Set(info.CongestedPercent)
		metricConnsMaxBackgroundPct.With(prometheus.Labels{"device_name": info.DeviceName}).Set(info.MaxBackgroundPercent)

		msgs := []string{}
		if info.CongestedPercent > c.congestedPercentAgainstThreshold {
			msgs = append(msgs, fmt.Sprintf("congested percent %.2f%% exceeds threshold %.2f%%", info.CongestedPercent, c.congestedPercentAgainstThreshold))
		}
		if info.MaxBackgroundPercent > c.maxBackgroundPercentAgainstThreshold {
			msgs = append(msgs, fmt.Sprintf("max background percent %.2f%% exceeds threshold %.2f%%", info.MaxBackgroundPercent, c.maxBackgroundPercentAgainstThreshold))
		}
		if len(msgs) == 0 {
			continue
		}

		if c.eventBucket == nil {
			continue
		}

		ib, err := json.Marshal(info)
		if err != nil {
			cr.err = err
			cr.reason = "error json encoding fuse connection info"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}

		ev := eventstore.Event{
			Time:    now.UTC(),
			Name:    "fuse_connections",
			Type:    string(apiv1.EventTypeCritical),
			Message: info.DeviceName + ": " + strings.Join(msgs, ", "),
			ExtraInfo: map[string]string{
				"data": string(ib),
			},
		}

		found, err := c.eventBucket.Find(c.ctx, ev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error finding event"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}
		if found == nil {
			continue
		}
		if err := c.eventBucket.Insert(c.ctx, ev); err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error inserting event"
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "successfully fuse connection(s)"
	log.Logger.Debugw(cr.reason, "count", len(cr.ConnectionInfos))

	return cr
}

type checkResult struct {
	ConnectionInfos []fuse.ConnectionInfo `json:"connection_infos"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	if len(cr.ConnectionInfos) == 0 {
		return "no FUSE connection found"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Device", "FS Type", "Congested %", "Max Background %"})
	for _, info := range cr.ConnectionInfos {
		table.Append([]string{info.DeviceName, info.Fstype, fmt.Sprintf("%.2f %%", info.CongestedPercent), fmt.Sprintf("%.2f %%", info.MaxBackgroundPercent)})
	}
	table.Render()

	return buf.String()
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{"data": string(b)}
	return apiv1.HealthStates{state}
}
