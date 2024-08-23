package tail

import (
	"bufio"
	"context"
	"fmt"
	"time"

	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"

	"github.com/nxadm/tail"
)

func NewFromCommand(ctx context.Context, commands [][]string, opts ...OpOption) (Streamer, error) {
	op := &Op{
		commands: commands,
	}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	p, err := process.New(op.commands, process.WithRunAsBashScript())
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
		op:    op,
		ctx:   ctx,
		proc:  p,
		lineC: make(chan Line, 200),
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
		matchedFilter *query_log_filter.Filter
	)

	for scanner.Scan() {
		select {
		case <-sr.ctx.Done():
			return
		default:
		}

		s = scanner.Text()
		ts, err = sr.op.parseTime([]byte(s))
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

		select {
		case sr.lineC <- Line{
			Line: &tail.Line{
				Text: s,
				Time: ts,
			},
			MatchedFilter: matchedFilter,
		}:
		default:
			log.Logger.Debugw("channel is full -- dropped output", "pid", sr.proc.PID())
		}
	}
}

func (sr *commandStreamer) waitCommand() {
	defer close(sr.lineC)
	select {
	case <-sr.ctx.Done():
	case <-sr.proc.Wait():
	}
}
