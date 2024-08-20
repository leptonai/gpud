package tail

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"time"

	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"
	"github.com/leptonai/gpud/log"

	"github.com/nxadm/tail"
)

func NewFromCommand(ctx context.Context, commands [][]string, opts ...OpOption) (Streamer, error) {
	op := &Op{
		commands: commands,
	}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	f, err := os.CreateTemp(os.TempDir(), "streamer-from-command*.txt")
	if err != nil {
		return nil, err
	}
	file := f.Name()

	if err := op.writeCommands(f); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "bash", file)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stdoutScanner := bufio.NewScanner(stdout)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stderrScanner := bufio.NewScanner(stderr)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	streamer := &commandStreamer{
		op:    op,
		ctx:   ctx,
		cmd:   cmd,
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
	cmd   *exec.Cmd
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
			log.Logger.Debugw("channel is full -- dropped output", "cmd", sr.cmd.String())
		}
	}
}

func (sr *commandStreamer) waitCommand() {
	defer close(sr.lineC)

	if err := sr.cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == -1 {
				if sr.ctx.Err() != nil {
					log.Logger.Debugw("command was terminated (exit code -1) by the root context cancellation", "cmd", sr.cmd.String(), "contextError", sr.ctx.Err())
				} else {
					log.Logger.Warnw("command was terminated (exit code -1) for unknown reasons", "cmd", sr.cmd.String())
				}
			} else {
				log.Logger.Warnw("command exited with non-zero status", "error", err, "cmd", sr.cmd.String(), "exitCode", exitErr.ExitCode())
			}
		} else {
			log.Logger.Warnw("error waiting for command to finish", "error", err, "cmd", sr.cmd.String())
		}
	} else {
		log.Logger.Debugw("command completed successfully")
	}
}
