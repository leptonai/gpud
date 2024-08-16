package dmesg

import (
	"context"
	"regexp"
	"sync"
	"time"

	query_log "github.com/leptonai/gpud/components/query/log"
	"github.com/leptonai/gpud/log"
)

var (
	defaultLogPollerOnce sync.Once
	defaultLogPoller     query_log.Poller
)

// only set once since it relies on the kube client and specific port
func createDefaultLogPoller(ctx context.Context, cfg Config) error {
	var err error
	defaultLogPollerOnce.Do(func() {
		defaultLogPoller, err = query_log.New(
			ctx,
			cfg.Log,
			ExtractTimeFromLogLine,
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

var regexForDmesgTime = regexp.MustCompile(`^\[([^\]]+)\]`)

// does not return error for now
// assume "dmesg --ctime" is used
// TODO: once stable return error
func ExtractTimeFromLogLine(line []byte) (time.Time, error) {
	matches := regexForDmesgTime.FindStringSubmatch(string(line))
	if len(matches) == 0 {
		log.Logger.Debugw("no timestamp matches found", "line", string(line))
		return time.Time{}, nil
	}

	s := matches[1]
	timestamp, err := time.Parse("Mon Jan 2 15:04:05 2006", s)
	if err != nil {
		log.Logger.Debugw("failed to parse timestamp", "line", string(line), "error", err)
		return time.Time{}, nil
	}
	return timestamp, nil
}
