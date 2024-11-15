package tail

import (
	"bufio"
	"context"
	"fmt"
	"time"

	query_log_common "github.com/leptonai/gpud/components/query/log/common"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"

	"github.com/nxadm/tail"
)

func NewFromCommand(ctx context.Context, commands [][]string, opts ...OpOption) (Streamer, error) {
	op := &Op{
		commands: commands,
	}
	if err := op.ApplyOpts(opts); err != nil {
		return nil, err
	}

	p, err := process.New(process.WithCommands(op.commands), process.WithRunAsBashScript())
	if err != nil {
		return nil, err
	}
	if err := p.Start(ctx); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-p.Wait():
		return nil, fmt.Errorf("command exited unexpectedly: %w", err)
	case <-time.After(50 * time.Millisecond):
	}

	stdoutScanner := bufio.NewScanner(p.StdoutReader())
	stderrScanner := bufio.NewScanner(p.StderrReader())

	streamer := &commandStreamer{
		op:           op,
		ctx:          ctx,
		proc:         p,
		lineC:        make(chan Line, 200),
		dedupEnabled: op.dedup,
	}

	if op.dedup {
		streamer.dedup = seenPool.Get().(*streamDeduper)
	}

	go streamer.pollLoops(stdoutScanner)
	go streamer.pollLoops(stderrScanner)
	go streamer.waitCommand()

	return streamer, nil
}

var _ Streamer = (*commandStreamer)(nil)

type commandStreamer struct {
	op    *Op
	ctx   context.Context
	proc  process.Process
	lineC chan Line

	dedupEnabled bool
	dedup        *streamDeduper
}

func (sr *commandStreamer) File() string {
	return ""
}

func (sr *commandStreamer) Commands() [][]string {
	return sr.op.commands
}

func (sr *commandStreamer) Line() <-chan Line {
	return sr.lineC
}

func (sr *commandStreamer) pollLoops(scanner *bufio.Scanner) {
	var (
		s             string
		ts            time.Time
		err           error
		shouldInclude bool
		matchedFilter *query_log_common.Filter
	)

	for scanner.Scan() {
		select {
		case <-sr.ctx.Done():
			return
		default:
		}

		s = scanner.Text()

		if sr.dedupEnabled {
			sr.dedup.mu.Lock()
			_, exists := sr.dedup.seen[s]
			if exists {
				sr.dedup.mu.Unlock()
				continue
			}
			sr.dedup.seen[s] = struct{}{}
			sr.dedup.mu.Unlock()
		}

		ts, err = sr.op.parseTime(scanner.Bytes())
		if err != nil {
			log.Logger.Warnw("error parsing time", "error", err)
			continue
		}
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		shouldInclude, matchedFilter, err = sr.op.applyFilter(s)
		if err != nil {
			log.Logger.Warnw("error applying filter", "error", err)
			continue
		}
		if !shouldInclude {
			continue
		}

		if sr.op.ProcessMatched != nil {
			sr.op.ProcessMatched(scanner.Bytes(), ts, matchedFilter)
		}

		lineToSend := Line{
			Line: &tail.Line{
				Text: s,
				Time: ts,
			},
			MatchedFilter: matchedFilter,
		}

		select {
		case <-sr.ctx.Done():
			return

		case sr.lineC <- lineToSend:

		default:
			log.Logger.Warnw("channel is full -- dropped output", "pid", sr.proc.PID())
		}
	}
}

func (sr *commandStreamer) waitCommand() {
	defer func() {
		close(sr.lineC)

		if sr.dedupEnabled {
			sr.dedup.mu.Lock()
			for k := range sr.dedup.seen {
				delete(sr.dedup.seen, k)
			}
			sr.dedup.mu.Unlock()
			seenPool.Put(sr.dedup)
		}
	}()

	select {
	case <-sr.ctx.Done():
	case <-sr.proc.Wait():
	}
}
