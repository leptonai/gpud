package fuse

import (
	"context"
	"sync"
	"time"

	fuse_id "github.com/leptonai/gpud/components/fuse/id"
	"github.com/leptonai/gpud/components/fuse/metrics"
	"github.com/leptonai/gpud/components/fuse/state"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/pkg/fuse"
	"github.com/leptonai/gpud/poller"
)

type Output struct {
	ConnectionInfos []fuse.ConnectionInfo `json:"connection_infos"`
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     poller.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = poller.New(
			fuse_id.Name,
			cfg.Query,
			CreateGet(cfg),
			nil,
		)
	})
}

func getDefaultPoller() poller.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) poller.GetFunc {
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

			prev, err := state.FindEvent(ctx, cfg.Query.State.DBRO, now.Unix(), info.DeviceName)
			if err != nil {
				return nil, err
			}
			if prev == nil {
				continue
			}

			if err := state.InsertEvent(ctx, cfg.Query.State.DBRW, state.Event{
				UnixSeconds:                          now.Unix(),
				DeviceName:                           info.DeviceName,
				CongestedPercentAgainstThreshold:     info.CongestedPercent,
				MaxBackgroundPercentAgainstThreshold: info.MaxBackgroundPercent,
			}); err != nil {
				return nil, err
			}

			if err := metrics.SetConnectionsCongestedPercent(ctx, info.DeviceName, info.CongestedPercent, now); err != nil {
				return nil, err
			}
			if err := metrics.SetConnectionsMaxBackgroundPercent(ctx, info.DeviceName, info.MaxBackgroundPercent, now); err != nil {
				return nil, err
			}

			foundDev[info.DeviceName] = info
		}

		return &Output{
			ConnectionInfos: infos,
		}, nil
	}
}
