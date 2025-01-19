package fd

import (
	"context"
	"sync"
	"time"

	fd_dmesg "github.com/leptonai/gpud/components/fd/dmesg"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	fd_state "github.com/leptonai/gpud/components/fd/state"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/dmesg"
	poller_config "github.com/leptonai/gpud/pkg/poller/config"
	poller_log "github.com/leptonai/gpud/pkg/poller/log"
	poller_log_common "github.com/leptonai/gpud/pkg/poller/log/common"
	poller_log_config "github.com/leptonai/gpud/pkg/poller/log/config"

	"k8s.io/utils/ptr"
)

const (
	// e.g.,
	// [...] VFS: file-max limit 1000000 reached
	//
	// ref.
	// https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
	EventVFSFileMaxLimitReached = "vfs_file_max_limit_reached"
)

func defaultDmesgFiltersForFileDescriptor() []*poller_log_common.Filter {
	return []*poller_log_common.Filter{
		{
			Name:            EventVFSFileMaxLimitReached,
			Regex:           ptr.To(fd_dmesg.RegexVFSFileMaxLimitReached),
			OwnerReferences: []string{fd_id.Name},
		},
	}
}

var (
	defaultLogPollerOnce sync.Once
	defaultLogPoller     poller_log.Poller
)

func setDefaultLogPoller(ctx context.Context, cfg poller_config.Config) error {
	var err error
	defaultLogPollerOnce.Do(func() {
		if err = fd_state.CreateTable(ctx, cfg.State.DBRW); err != nil {
			return
		}
		go func() {
			dur := fd_state.DefaultRetentionPeriod
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(dur):
					now := time.Now().UTC()
					before := now.Add(-dur)

					purged, err := fd_state.Purge(ctx, cfg.State.DBRW, fd_state.WithBefore(before))
					if err != nil {
						log.Logger.Warnw("failed to delete events", "error", err)
					} else {
						log.Logger.Debugw("deleted events", "before", before, "purged", purged)
					}
				}
			}
		}()

		var cmds *dmesg.Commands
		cmds, err = dmesg.GetCommands(ctx)
		if err != nil {
			return
		}

		logCfg := poller_log_config.Config{
			PollerConfig:  cfg,
			BufferSize:    poller_log_config.DefaultBufferSize,
			Commands:      ptr.To(cmds.WatchCommands),
			SelectFilters: defaultDmesgFiltersForFileDescriptor(),
			ExtractTime:   cmds.ParseTimeFunc,
			ProcessMatched: func(parsedTime time.Time, line []byte, filter *poller_log_common.Filter) {
				if ierr := fd_state.InsertEvent(ctx, cfg.State.DBRW, fd_state.Event{
					UnixSeconds:  parsedTime.Unix(),
					DataSource:   "dmesg",
					EventType:    filter.Name,
					EventDetails: string(line),
				}); ierr != nil {
					log.Logger.Errorw("failed to insert event", "error", ierr)
				}
			},
		}

		defaultLogPoller, err = poller_log.New(
			ctx,
			logCfg,
		)
	})

	return err
}

func getDefaultLogPoller() poller_log.Poller {
	return defaultLogPoller
}
