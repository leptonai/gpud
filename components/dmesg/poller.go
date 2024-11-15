package dmesg

import (
	"context"
	"sync"

	query_log "github.com/leptonai/gpud/components/query/log"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
)

var (
	defaultLogPollerOnce sync.Once
	defaultLogPoller     query_log.Poller
)

// only set once since it relies on the kube client and specific port
func createDefaultLogPoller(ctx context.Context, cfg Config, processMatched query_log_common.ProcessMatchedFunc) error {
	var err error
	defaultLogPollerOnce.Do(func() {
		defaultLogPoller, err = query_log.New(
			ctx,
			cfg.Log,
			pkg_dmesg.ParseISOtimeWithError,
			processMatched,
		)
		if err != nil {
			panic(err)
		}
	})
	return err
}

func GetDefaultLogPoller() query_log.Poller {
	return defaultLogPoller
}
