// Package fuse monitors the FUSE (Filesystem in Userspace).
package fuse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
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
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
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

	lastMu   sync.RWMutex
	lastData *Data
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

	if gpudInstance.EventStore != nil && runtime.GOOS == "linux" {
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
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getLastHealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket != nil {
		return c.eventBucket.Get(ctx, since)
	}
	return nil, nil
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

	d := &Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = d
		c.lastMu.Unlock()
	}()

	infos, err := c.listConnectionsFunc()
	if err != nil {
		d.err = err
		d.health = apiv1.StateTypeUnhealthy
		d.reason = fmt.Sprintf("error listing fuse connections %v", err)
		return d
	}

	now := time.Now().UTC()

	foundDev := make(map[string]fuse.ConnectionInfo)
	for _, info := range infos {
		// to dedup fuse connection stats by device name
		if _, ok := foundDev[info.DeviceName]; ok {
			continue
		}
		foundDev[info.DeviceName] = info

		metricConnsCongestedPct.With(prometheus.Labels{pkgmetrics.MetricLabelKey: info.DeviceName}).Set(info.CongestedPercent)
		metricConnsMaxBackgroundPct.With(prometheus.Labels{pkgmetrics.MetricLabelKey: info.DeviceName}).Set(info.MaxBackgroundPercent)

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

		ib, err := info.JSON()
		if err != nil {
			log.Logger.Errorw("error getting json of fuse connection info", "error", err)
			continue
		}
		ev := apiv1.Event{
			Time:    metav1.Time{Time: now.UTC()},
			Name:    "fuse_connections",
			Type:    apiv1.EventTypeCritical,
			Message: info.DeviceName + ": " + strings.Join(msgs, ", "),
			DeprecatedExtraInfo: map[string]string{
				"data":     string(ib),
				"encoding": "json",
			},
		}

		found, err := c.eventBucket.Find(c.ctx, ev)
		if err != nil {
			d.err = err
			d.health = apiv1.StateTypeUnhealthy
			d.reason = fmt.Sprintf("error finding event %v", err)
			return d
		}
		if found == nil {
			continue
		}
		if err := c.eventBucket.Insert(c.ctx, ev); err != nil {
			d.err = err
			d.health = apiv1.StateTypeUnhealthy
			d.reason = fmt.Sprintf("error inserting event %v", err)
			return d
		}
	}

	d.health = apiv1.StateTypeHealthy
	d.reason = fmt.Sprintf("found %d fuse connection(s)", len(d.ConnectionInfos))

	return d
}

type Data struct {
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

func (d *Data) String() string {
	if d == nil {
		return ""
	}
	if len(d.ConnectionInfos) == 0 {
		return "no FUSE connection found"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetHeader([]string{"Device", "FS Type", "Congested % %"})
	for _, info := range d.ConnectionInfos {
		table.Append([]string{info.DeviceName, info.Fstype, fmt.Sprintf("%.2f %%", info.CongestedPercent), fmt.Sprintf("%.2f %%", info.MaxBackgroundPercent)})
	}
	table.Render()

	return buf.String()
}

func (d *Data) Summary() string {
	if d == nil {
		return ""
	}
	return d.reason
}

func (d *Data) HealthState() apiv1.HealthStateType {
	if d == nil {
		return ""
	}
	return d.health
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getLastHealthStates() apiv1.HealthStates {
	if d == nil {
		return apiv1.HealthStates{
			{
				Name:   Name,
				Health: apiv1.StateTypeHealthy,
				Reason: "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),
		Health: d.health,
	}

	b, _ := json.Marshal(d)
	state.DeprecatedExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return apiv1.HealthStates{state}
}
