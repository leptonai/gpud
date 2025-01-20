package fd

import (
	"context"
	"sync"
	"time"

	fd_dmesg "github.com/leptonai/gpud/components/fd/dmesg"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	"github.com/leptonai/gpud/components/state"
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
				ev := state.Event{
					Timestamp:    parsedTime.Unix(),
					EventType:    filter.Name,
					DataSource:   "dmesg",
					EventDetails: string(line),
				}

				found, err := state.FindEvent(ctx, cfg.State.DBRO, ev)
				if err != nil {
					log.Logger.Errorw("failed to find event", "error", err)
				}
				if found {
					log.Logger.Debugw("event already exists", "event", ev)
					return
				}

				if err := state.InsertEvent(ctx, cfg.State.DBRW, ev); err != nil {
					log.Logger.Errorw("failed to insert event", "error", err)
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
