// Package fabricmanagerlog implements the fabric manager log poller.
package fabricmanagerlog

import (
	"context"
	"regexp"
	"sync"
	"time"

	query_log "github.com/leptonai/gpud/components/query/log"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
	"github.com/leptonai/gpud/log"
)

var (
	defaultLogPollerOnce sync.Once
	defaultLogPoller     query_log.Poller
)

func CreateDefaultPoller(ctx context.Context, cfg query_log_config.Config) error {
	var err error
	defaultLogPollerOnce.Do(func() {
		defaultLogPoller, err = query_log.New(
			ctx,
			cfg,
			ExtractTimeFromLogLine,
			nil,
		)
		if err != nil {
			panic(err)
		}
	})
	return err
}

func GetDefaultPoller() query_log.Poller {
	return defaultLogPoller
}

var regexForFabricmanagerLog = regexp.MustCompile(`^\[([^\]]+)\]`)

// does not return error for now
// example log line: "[May 02 2024 18:41:23] [INFO] [tid 404868] Abort CUDA jobs when FM exits = 1"
// TODO: once stable return error
func ExtractTimeFromLogLine(line []byte) (time.Time, error) {
	matches := regexForFabricmanagerLog.FindStringSubmatch(string(line))
	if len(matches) == 0 {
		log.Logger.Debugw("no timestamp matches found", "line", string(line))
		return time.Time{}, nil
	}

	s := matches[1]
	timestamp, err := time.Parse("Jan 02 2006 15:04:05", s)
	if err != nil {
		log.Logger.Debugw("failed to parse timestamp", "line", string(line), "error", err)
		return time.Time{}, nil
	}
	return timestamp, nil
}
