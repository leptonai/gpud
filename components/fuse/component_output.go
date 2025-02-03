package fuse

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	events_db "github.com/leptonai/gpud/components/db"
	fuse_id "github.com/leptonai/gpud/components/fuse/id"
	"github.com/leptonai/gpud/components/fuse/metrics"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/pkg/fuse"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Output struct {
	ConnectionInfos []fuse.ConnectionInfo `json:"connection_infos"`
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config, eventsStore events_db.Store) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			fuse_id.Name,
			cfg.Query,
			CreateGet(cfg, eventsStore),
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config, eventsStore events_db.Store) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(fuse_id.Name)
			} else {
				components_metrics.SetGetSuccess(fuse_id.Name)
			}
		}()

		infos, err := fuse.ListConnections()
		if err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		nowUTC := float64(now.Unix())
		metrics.SetLastUpdateUnixSeconds(nowUTC)

		foundDev := make(map[string]fuse.ConnectionInfo)
		for _, info := range infos {
			// to dedup fuse connection stats by device name
			if _, ok := foundDev[info.DeviceName]; ok {
				continue
			}

			if err := metrics.SetConnectionsCongestedPercent(ctx, info.DeviceName, info.CongestedPercent, now); err != nil {
				return nil, err
			}
			if err := metrics.SetConnectionsMaxBackgroundPercent(ctx, info.DeviceName, info.MaxBackgroundPercent, now); err != nil {
				return nil, err
			}

			msgs := []string{}
			if info.CongestedPercent > cfg.CongestedPercentAgainstThreshold {
				msgs = append(msgs, fmt.Sprintf("congested percent against threshold %.2f exceeds threshold %.2f", info.CongestedPercent, cfg.CongestedPercentAgainstThreshold))
			}
			if info.MaxBackgroundPercent > cfg.MaxBackgroundPercentAgainstThreshold {
				msgs = append(msgs, fmt.Sprintf("max background percent against threshold %.2f exceeds threshold %.2f", info.MaxBackgroundPercent, cfg.MaxBackgroundPercentAgainstThreshold))
			}
			if len(msgs) == 0 {
				continue
			}

			ib, err := info.JSON()
			if err != nil {
				continue
			}
			ev := components.Event{
				Time:    metav1.Time{Time: now.UTC()},
				Name:    "fuse_connections",
				Type:    common.EventTypeCritical,
				Message: info.DeviceName + ": " + strings.Join(msgs, ", "),
				ExtraInfo: map[string]string{
					"data":     string(ib),
					"encoding": "json",
				},
			}

			found, err := eventsStore.Find(ctx, ev)
			if err != nil {
				return nil, err
			}
			if found == nil {
				continue
			}
			if err := eventsStore.Insert(ctx, ev); err != nil {
				return nil, err
			}

			foundDev[info.DeviceName] = info
		}

		return &Output{
			ConnectionInfos: infos,
		}, nil
	}
}
