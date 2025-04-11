// Package fuse monitors the FUSE (Filesystem in Userspace).
package fuse

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/fuse"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const Name = "fuse"

const (
	DefaultCongestedPercentAgainstThreshold     = float64(90)
	DefaultMaxBackgroundPercentAgainstThreshold = float64(80)
)

var _ apiv1.Component = &component{}

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

// CongestedPercentAgainstThreshold is the percentage of the FUSE connections waiting
// at which we consider the system to be congested.
//
// MaxBackgroundPercentAgainstThreshold is the percentage of the FUSE connections waiting
// at which we consider the system to be congested.
func New(ctx context.Context, congestedPercentAgainstThreshold float64, maxBackgroundPercentAgainstThreshold float64, eventStore eventstore.Store) (apiv1.Component, error) {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		return nil, err
	}

	if congestedPercentAgainstThreshold == 0 {
		congestedPercentAgainstThreshold = DefaultCongestedPercentAgainstThreshold
	}
	if maxBackgroundPercentAgainstThreshold == 0 {
		maxBackgroundPercentAgainstThreshold = DefaultMaxBackgroundPercentAgainstThreshold
	}

	cctx, ccancel := context.WithCancel(ctx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		congestedPercentAgainstThreshold:     congestedPercentAgainstThreshold,
		maxBackgroundPercentAgainstThreshold: maxBackgroundPercentAgainstThreshold,

		listConnectionsFunc: fuse.ListConnections,

		eventBucket: eventBucket,
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			c.CheckOnce()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) States(ctx context.Context) ([]apiv1.State, error) {
	c.lastMu.RLock()
	lastData := c.lastData
	c.lastMu.RUnlock()
	return lastData.getStates()
}

func (c *component) Events(ctx context.Context, since time.Time) ([]apiv1.Event, error) {
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

// CheckOnce checks the current pods
// run this periodically
func (c *component) CheckOnce() {
	log.Logger.Infow("checking fuse")
	d := Data{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastData = &d
		c.lastMu.Unlock()
	}()

	infos, err := c.listConnectionsFunc()
	if err != nil {
		d.err = err
		d.healthy = false
		d.reason = fmt.Sprintf("error listing fuse connections %v", err)
		return
	}

	now := time.Now().UTC()

	foundDev := make(map[string]fuse.ConnectionInfo)
	for _, info := range infos {
		// to dedup fuse connection stats by device name
		if _, ok := foundDev[info.DeviceName]; ok {
			continue
		}

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
			ExtraInfo: map[string]string{
				"data":     string(ib),
				"encoding": "json",
			},
		}

		found, err := c.eventBucket.Find(c.ctx, ev)
		if err != nil {
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error finding event %v", err)
			return
		}
		if found == nil {
			continue
		}
		if err := c.eventBucket.Insert(c.ctx, ev); err != nil {
			d.err = err
			d.healthy = false
			d.reason = fmt.Sprintf("error inserting event %v", err)
			return
		}

		foundDev[info.DeviceName] = info
	}

	d.healthy = true
	d.reason = fmt.Sprintf("found %d fuse connection(s)", len(d.ConnectionInfos))
}

type Data struct {
	ConnectionInfos []fuse.ConnectionInfo `json:"connection_infos"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	healthy bool
	// tracks the reason of the last check
	reason string
}

func (d *Data) getError() string {
	if d == nil || d.err == nil {
		return ""
	}
	return d.err.Error()
}

func (d *Data) getStates() ([]apiv1.State, error) {
	if d == nil {
		return []apiv1.State{
			{
				Name:    Name,
				Health:  apiv1.StateHealthy,
				Healthy: true,
				Reason:  "no data yet",
			},
		}, nil
	}

	state := apiv1.State{
		Name:   Name,
		Reason: d.reason,
		Error:  d.getError(),

		Healthy: d.healthy,
		Health:  apiv1.StateHealthy,
	}
	if !d.healthy {
		state.Health = apiv1.StateUnhealthy
	}

	b, _ := json.Marshal(d)
	state.ExtraInfo = map[string]string{
		"data":     string(b),
		"encoding": "json",
	}
	return []apiv1.State{state}, nil
}
