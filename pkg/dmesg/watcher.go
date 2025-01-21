package dmesg

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/process"
)

var DefaultWatchCommands = []string{
	"dmesg --decode --time-format=iso --nopager --buffer-size 163920 -w || true",

	// run last commands as fallback, in case "dmesg -w" flag only works in some machines
	"dmesg --decode --time-format=iso --nopager --buffer-size 163920 -W || true",
}

type LogLine struct {
	Timestamp time.Time
	Facility  string
	Level     string
	Content   string

	// Error is set when a dmesg command fails.
	Error string
}

type Watcher interface {
	// Watch returns a channel that emits log lines.
	// The channel is closed on (1) process exit, (2) on calling "Close" method
	Watch() <-chan LogLine
	// Closes the existing/ongoing watch/stream routines.
	// Safe to call multiple times.
	Close()
}

func NewWatcher() (Watcher, error) {
	return newWatcher(DefaultWatchCommands...)
}

func newWatcher(cmds ...string) (Watcher, error) {
	if len(cmds) == 0 {
		return nil, errors.New("no commands provided")
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := watch(ctx, cmds...)
	if err != nil {
		cancel()
		return nil, err
	}
	return &watcher{ch: ch, cancel: cancel}, nil
}

type watcher struct {
	ch     <-chan LogLine
	cancel context.CancelFunc
}

func (w *watcher) Watch() <-chan LogLine {
	return w.ch
}

func (w *watcher) Close() {
	w.cancel()
}

func watch(
	ctx context.Context,
	cmds ...string,
) (<-chan LogLine, error) {
	ch := make(chan LogLine, 1000)
	p, err := process.New(
		process.WithCommand(cmds...),
		process.WithRunAsBashScript(), // need to run as bash script when dmesg commands are complicated
	)
	if err != nil {
		return nil, err
	}
	if err := p.Start(ctx); err != nil {
		return nil, err
	}
	go wait(ctx, p)
	go read(ctx, p, ch)
	return ch, nil
}

func wait(ctx context.Context, p process.Process) {
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

func read(ctx context.Context, p process.Process, ch chan<- LogLine) {
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

			logLine := parseDmesgLine(line)
			select {
			case <-ctx.Done():
				return
			case ch <- logLine:
			default:
				log.Logger.Warnw("failed to send event -- dropped")
			}
		}),
		process.WithWaitForCmd(),
	); err != nil {
		select {
		case <-ctx.Done():
			return
		case ch <- LogLine{
			Timestamp: time.Now().UTC(),
			Error:     fmt.Sprintf("reading output failed: %v", err),
		}:
		}
	}
}

const isoFormat = "2006-01-02T15:04:05,999999-07:00"

// parses the timestamp from "dmesg --time-format=iso" output lines.
// "The definition of the iso timestamp is: YYYY-MM-DD<T>HH:MM:SS,<microseconds>←+><timezone offset from UTC>."
func parseDmesgLine(line string) LogLine {
	logLine := LogLine{Timestamp: time.Now().UTC(), Content: line}

	// grep the first numeric characters to truncate the decodePfx
	// e.g., "kern  :warn  : 2025-01-21T04:41:44..."
	yidx := findISOTimestampIndex(line)
	if yidx == -1 {
		return logLine
	}

	decodedLine := strings.TrimSpace(line[:yidx])
	decoded := []string{}
	for _, d := range strings.Split(decodedLine, ":") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		decoded = append(decoded, d)
	}
	if len(decoded) > 0 {
		logLine.Facility = decoded[0]
	}
	if len(decoded) > 1 {
		logLine.Level = decoded[1]
	}

	line = line[yidx:]

	parsedTime, err := time.Parse(isoFormat, line[:len(isoFormat)])
	if err != nil {
		log.Logger.Warnw("failed to parse timestamp", "err", err, "line", line)
		logLine.Content = line
		return logLine
	}

	if len(line) < len(isoFormat) {
		logLine.Content = line
		return logLine
	}

	extractedLine := strings.TrimSpace(line[len(isoFormat):])
	logLine.Timestamp = parsedTime
	logLine.Content = extractedLine
	return logLine
}

var isoTsRegex = regexp.MustCompile(`\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])T([01]\d|2[0-3]):([0-5]\d):([0-5]\d)`)

func findISOTimestampIndex(s string) int {
	loc := isoTsRegex.FindStringIndex(s)
	if loc == nil {
		return -1
	}
	return loc[0]
}
