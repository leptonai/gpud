package streamer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

type LogLine struct {
	Time    time.Time
	Content string

	// Error is set when a watch command fails
	Error error
}

type cmdStreamer interface {
	// watch returns a channel that emits log lines.
	// The channel is closed on (1) process exit, (2) on calling "Close" method
	watch() <-chan LogLine
	// closes the existing/ongoing watch/stream routines.
	// Safe to call multiple times.
	close()
}

func newCmdStreamer(cmds [][]string, parseLogFunc func(line string) LogLine) (cmdStreamer, error) {
	if len(cmds) == 0 {
		return nil, errors.New("no commands provided")
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := streamCommandOutputs(ctx, cmds, parseLogFunc, defaultCacheExpiration, defaultCachePurgeInterval)
	if err != nil {
		cancel()
		return nil, err
	}
	return &cmdStreamerImpl{ch: ch, cancel: cancel}, nil
}

type cmdStreamerImpl struct {
	ch     <-chan LogLine
	cancel context.CancelFunc
}

func (w *cmdStreamerImpl) watch() <-chan LogLine {
	return w.ch
}

func (w *cmdStreamerImpl) close() {
	w.cancel()
}

func streamCommandOutputs(
	ctx context.Context,
	cmds [][]string,
	parseLogFunc func(line string) LogLine,
	cacheExpiration time.Duration,
	cachePurgeInterval time.Duration,
) (<-chan LogLine, error) {
	// initial 'tail' command may return >1k lines, buffer 3k to minimize the event loss
	ch := make(chan LogLine, 3000)

	opts := []process.OpOption{}
	for _, cmd := range cmds {
		opts = append(opts, process.WithCommand(cmd...))
	}

	// need to run as bash script when 'tail' commands are complicated
	opts = append(opts, process.WithRunAsBashScript())

	p, err := process.New(opts...)
	if err != nil {
		return nil, err
	}
	if err := p.Start(ctx); err != nil {
		return nil, err
	}
	go waitProcess(ctx, p)

	deduper := newDeduper(cacheExpiration, cachePurgeInterval)
	go readProcessOutputs(ctx, p, deduper, parseLogFunc, ch)

	return ch, nil
}

func waitProcess(ctx context.Context, p process.Process) {
	defer func() {
		// we don't close until either context is done or the process is exited
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()
	select {
	case <-ctx.Done():
	case <-p.Wait():
	}
}

func readProcessOutputs(ctx context.Context, p process.Process, deduper *deduper, parseLogFunc func(line string) LogLine, ch chan<- LogLine) {
	defer close(ch)

	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			if len(line) == 0 {
				return
			}

			parsed := parseLogFunc(line)
			if occurrences := deduper.addCache(parsed); occurrences > 1 {
				log.Logger.Debugw("skipping duplicate log line", "occurrences", occurrences, "timestamp", parsed.Time, "line", parsed.Content)
				return
			}

			select {
			case <-ctx.Done():
				return
			case ch <- parsed:
			default:
				log.Logger.Warnw("failed to send event -- dropped", "dropped", parsed)
			}
		}),

		// larger to avoid output truncation
		// when 'tail' command exits for some reason
		process.WithInitialBufferSize(16384),

		process.WithWaitForCmd(),
	); err != nil {
		select {
		case <-ctx.Done():
			return
		case ch <- LogLine{
			Time:  time.Now().UTC(),
			Error: fmt.Errorf("reading output failed: %v", err),
		}:
			log.Logger.Warnw("failed to read from output", "err", err)
		}
	}
}
