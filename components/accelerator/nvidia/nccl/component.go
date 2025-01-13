// Package nccl monitors the NCCL status.
// Optional, enabled if the host has NVIDIA GPUs.
package nccl

import (
	"context"
	"runtime"
	"strconv"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_nccl_id "github.com/leptonai/gpud/components/accelerator/nvidia/nccl/id"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	common_dmesg "github.com/leptonai/gpud/components/common/dmesg"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
)

func New(ctx context.Context, cfg Config) components.Component {
	cfg.Query.SetDefaultsIfNotSet()

	cctx, ccancel := context.WithCancel(ctx)
	nvidia_query.SetDefaultPoller(cfg.Query.State.DBRW, cfg.Query.State.DBRO)
	nvidia_query.GetDefaultPoller().Start(cctx, cfg.Query, nvidia_nccl_id.Name)

	if runtime.GOOS == "linux" {
		if perr := common_dmesg.SetDefaultLogPoller(ctx, cfg.Query.State.DBRW, cfg.Query.State.DBRO); perr != nil {
			log.Logger.Warnw("failed to set default log poller", "error", perr)
		} else {
			common_dmesg.GetDefaultLogPoller().Start(cctx, cfg.Query, common_dmesg.Name)
		}
	}

	return &component{
		rootCtx: ctx,
		cancel:  ccancel,
		poller:  nvidia_query.GetDefaultPoller(),
	}
}

var _ components.Component = (*component)(nil)

type component struct {
	rootCtx context.Context
	cancel  context.CancelFunc
	poller  query.Poller
}

func (c *component) Name() string { return nvidia_nccl_id.Name }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	return []components.State{
		{
			Healthy: true,
			Reason:  "no issue",
		},
	}, nil
}

const (
	// repeated messages may indicate GPU communication issues, which may happen due to fabric manager issues
	// e.g.,
	// [Thu Oct 10 03:06:53 2024] pt_main_thread[2536443]: segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2[7f7c7ac00000+d3d3000]
	EventNameNCCLSegfaultInLibncclFromDmesg = "nccl_segfault_in_libnccl_from_dmesg"

	EventKeyNCCLSegfaultInLibncclFromDmesgUnixSeconds = "unix_seconds"
	EventKeyNCCLSegfaultInLibncclFromDmesgLogLine     = "log_line"
)

func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}

	logItems, err := common_dmesg.GetDefaultLogPoller().Find(since)
	if err != nil {
		return nil, err
	}

	events := make([]components.Event, 0)
	for _, logItem := range logItems {
		if logItem.Error != nil {
			continue
		}
		if logItem.Matched == nil {
			continue
		}
		if logItem.Matched.Name != common_dmesg.EventNvidiaNCCLSegfaultInLibnccl {
			continue
		}

		// "TailScanMatched" are sorted by the time from new to old
		// thus keeping the first 30 latest, to prevent too many events
		if len(events) > 30 {
			break
		}

		events = append(events, components.Event{
			Time: logItem.Time,
			Name: EventNameNCCLSegfaultInLibncclFromDmesg,
			Type: components.EventTypeCritical,
			ExtraInfo: map[string]string{
				EventKeyNCCLSegfaultInLibncclFromDmesgUnixSeconds: strconv.FormatInt(logItem.Time.Unix(), 10),
				EventKeyNCCLSegfaultInLibncclFromDmesgLogLine:     logItem.Line,
			},
		})
	}

	return events, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	// safe to call stop multiple times
	_ = c.poller.Stop(nvidia_nccl_id.Name)

	if runtime.GOOS == "linux" {
		common_dmesg.GetDefaultLogPoller().Stop(common_dmesg.Name)
	}

	c.cancel()
	return nil
}
