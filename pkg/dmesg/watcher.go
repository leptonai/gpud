package dmesg

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

var DefaultDmesgScanCommands = [][]string{
	{"dmesg --decode --time-format=iso --nopager --buffer-size 163920"},
}

var DefaultWatchCommands = [][]string{
	{"dmesg --decode --time-format=iso --nopager --buffer-size 163920 -w || true"},

	// in case "dmesg -w" fails, tail the existing dmesg buffer
	{"dmesg --decode --time-format=iso --nopager --buffer-size 163920 || true"},

	// run last commands as fallback, in case "dmesg -w" flag only works in some machines
	{"dmesg --decode --time-format=iso --nopager --buffer-size 163920 -W || true"},
}

type LogLine struct {
	Timestamp time.Time
	Facility  string
	Level     string
	Content   string

	// Error is set when a dmesg command fails.
	Error string
}

func (l LogLine) IsEmpty() bool {
	return l.Timestamp.IsZero() && l.Facility == "" && l.Level == "" && l.Content == "" && l.Error == ""
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
	return NewWatcherWithCommands(DefaultWatchCommands)
}

const (
	DefaultCacheExpiration    = 5 * time.Minute
	DefaultCachePurgeInterval = 10 * time.Minute
)

func NewWatcherWithCommands(cmds [][]string) (Watcher, error) {
	if len(cmds) == 0 {
		return nil, errors.New("no commands provided")
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := watch(ctx, cmds, DefaultCacheExpiration, DefaultCachePurgeInterval)
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
	cmds [][]string,
	cacheExpiration time.Duration,
	cachePurgeInterval time.Duration,
) (<-chan LogLine, error) {
	// initial dmesg command may return >1k lines, buffer 3k to minimize the event loss
	ch := make(chan LogLine, 3000)

	opts := []process.OpOption{}
	for _, cmd := range cmds {
		opts = append(opts, process.WithCommand(cmd...))
	}

	// need to run as bash script when dmesg commands are complicated
	opts = append(opts, process.WithRunAsBashScript())

	p, err := process.New(opts...)
	if err != nil {
		return nil, err
	}
	if err := p.Start(ctx); err != nil {
		return nil, err
	}
	go wait(ctx, p)
	go read(ctx, p, cacheExpiration, cachePurgeInterval, ch)
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

func read(ctx context.Context, p process.Process, cacheExpiration time.Duration, cachePurgeInterval time.Duration, ch chan<- LogLine) {
	defer close(ch)

	// dedup by second
	deduper := newDeduper(cacheExpiration, cachePurgeInterval)

	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			if len(line) == 0 {
				return
			}

			parsed := ParseDmesgLine(line)
			if occurrences := deduper.addCache(parsed); occurrences > 1 {
				log.Logger.Warnw("skipping duplicate log line", "occurrences", occurrences, "timestamp", parsed.Timestamp, "line", parsed.Content)
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

		// default buffer size of "dmesg" is 16384 bytes
		// set initial buffer size to 16384 bytes
		// larger than default 4KB to avoid output truncation
		// when dmesg command exits for some reason
		// ref. https://linux.die.net/man/8/dmesg
		process.WithInitialBufferSize(16384),

		process.WithWaitForCmd(),
	); err != nil {
		select {
		case <-ctx.Done():
			return
		case ch <- LogLine{
			Timestamp: time.Now().UTC(),
			Error:     fmt.Sprintf("reading output failed: %v", err),
		}:
			log.Logger.Warnw("failed to read dmesg output", "err", err)
		}
	}
}

// parses the timestamp from "dmesg --time-format=iso" output lines.
// "The definition of the iso timestamp is: YYYY-MM-DD<T>HH:MM:SS,<microseconds>←+><timezone offset from UTC>."
func ParseDmesgLine(line string) LogLine {
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
	logLine.Timestamp = parsedTime

	if len(line) < len(isoFormat) {
		logLine.Content = line
		return logLine
	}

	extractedLine := strings.TrimSpace(line[len(isoFormat):])
	logLine.Content = extractedLine
	return logLine
}

const isoFormat = "2006-01-02T15:04:05,999999-07:00"

// shorter "2025-01-17T15:36:11" should not match
// only "2025-01-17T15:36:17,304997+00:00" should match
var isoTsRegex = regexp.MustCompile(`\d{4}-(0[1-9]|1[0-2])-(0[1-9]|[12]\d|3[01])T([01]\d|2[0-3]):([0-5]\d):([0-5]\d),\d{6}[+-]\d{2}:\d{2}`)

func findISOTimestampIndex(s string) int {
	loc := isoTsRegex.FindStringIndex(s)
	if loc == nil {
		return -1
	}
	return loc[0]
}
