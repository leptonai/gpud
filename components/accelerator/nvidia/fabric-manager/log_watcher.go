package fabricmanager

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	cache "github.com/patrickmn/go-cache"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

type logLineProcessor struct {
	ctx         context.Context
	w           watcher
	matchFunc   matchFunc
	eventsStore events_db.Store
}

type matchFunc func(line string) (eventName string, message string)

func newLogLineProcessor(
	ctx context.Context,
	w watcher,
	matchFunc matchFunc,
	eventsStore events_db.Store,
) *logLineProcessor {
	llp := &logLineProcessor{
		ctx:         ctx,
		w:           w,
		matchFunc:   matchFunc,
		eventsStore: eventsStore,
	}
	go llp.watch()
	return llp
}

func (llp *logLineProcessor) watch() {
	ch := llp.w.watch()
	for {
		select {
		case <-llp.ctx.Done():
			return
		case line, open := <-ch:
			if !open {
				return
			}

			ev := components.Event{
				Time: metav1.Time{Time: line.ts.UTC()},
				Type: common.EventTypeWarning,
				ExtraInfo: map[string]string{
					"log_line": line.content,
				},
			}

			ev.Name, ev.Message = llp.matchFunc(line.content)
			if ev.Name == "" {
				continue
			}

			// lookup to prevent duplicate event insertions
			cctx, ccancel := context.WithTimeout(llp.ctx, 15*time.Second)
			found, err := llp.eventsStore.Find(
				cctx,
				components.Event{
					Time:    ev.Time,
					Name:    ev.Name,
					Message: ev.Message,
					Type:    ev.Type,
				},
			)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to find event", "eventName", ev.Name, "eventType", ev.Type, "error", err)
			}
			if found != nil {
				continue
			}

			// insert event
			cctx, ccancel = context.WithTimeout(llp.ctx, 15*time.Second)
			err = llp.eventsStore.Insert(cctx, ev)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to insert event", "error", err)
			} else {
				log.Logger.Infow("successfully inserted event", "event", ev.Name)
			}
		}
	}
}

func (llp *logLineProcessor) getEvents(ctx context.Context, since time.Time) ([]components.Event, error) {
	return llp.eventsStore.Get(ctx, since)
}

func (llp *logLineProcessor) close() {
	llp.w.close()
}

type logLine struct {
	ts      time.Time
	content string
	// err is set when a watch command fails
	err error
}

const fabricmanagerLogTimeFormat = "Jan 02 2006 15:04:05"

var (
	fabricmanagerLogTimeFormatN = len(fabricmanagerLogTimeFormat) + 2 // [ ]
	regexForFabricmanagerLog    = regexp.MustCompile(`^\[([^\]]+)\]`)
)

func parseLogLine(line string) logLine {
	logLine := logLine{ts: time.Now().UTC(), content: line}

	matches := regexForFabricmanagerLog.FindStringSubmatch(line)
	if len(matches) == 0 {
		log.Logger.Warnw("no timestamp matches found", "line", line)
		logLine.err = errors.New("no timestamp matches found")
		return logLine
	}

	s := matches[1]
	parsedTime, err := time.Parse(fabricmanagerLogTimeFormat, s)
	if err != nil {
		log.Logger.Warnw("failed to parse timestamp", "line", line, "error", err)
		logLine.err = err
		return logLine
	}
	logLine.ts = parsedTime

	if len(line) <= fabricmanagerLogTimeFormatN {
		return logLine
	}

	logLine.content = strings.TrimSpace(line[fabricmanagerLogTimeFormatN:])
	return logLine
}

func (l logLine) cacheKey() string {
	return fmt.Sprintf("%d-%s", l.ts.Unix(), l.content)
}

// caches the log lines and its frequencies
type deduper struct {
	cache *cache.Cache
}

func newDeduper(cacheExpiration time.Duration, cachePurgeInterval time.Duration) *deduper {
	return &deduper{
		cache: cache.New(cacheExpiration, cachePurgeInterval),
	}
}

// addCache returns the current count of occurrences of the log line, found in the cache
// Returns 1 if the log line was not in the cache thus first occurrence.
// Returns 2 if the log line was in the cache once before, thus second occurrence.
func (d *deduper) addCache(l logLine) int {
	k := l.cacheKey()

	var freq int
	cur, found := d.cache.Get(k)
	if !found {
		freq = 1
	} else {
		v, _ := cur.(int)
		freq = v + 1
	}

	d.cache.Set(k, freq, cache.DefaultExpiration)
	return freq
}

type watcher interface {
	// watch returns a channel that emits log lines.
	// The channel is closed on (1) process exit, (2) on calling "Close" method
	watch() <-chan logLine
	// closes the existing/ongoing watch/stream routines.
	// Safe to call multiple times.
	close()
}

const (
	// default log file location is LOG_FILE_NAME=∕var∕log∕fabricmanager.log
	// ref. https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf
	defaultLogFileName = "/var/log/fabricmanager.log"

	defaultCacheExpiration    = 5 * time.Minute
	defaultCachePurgeInterval = 10 * time.Minute
)

var defaultWatchCommands = [][]string{
	{fmt.Sprintf("tail -f %s || true", defaultLogFileName)},
}

func newWatcher(cmds [][]string) (watcher, error) {
	if len(cmds) == 0 {
		return nil, errors.New("no commands provided")
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := watch(ctx, cmds, defaultCacheExpiration, defaultCachePurgeInterval)
	if err != nil {
		cancel()
		return nil, err
	}
	return &watcherImpl{ch: ch, cancel: cancel}, nil
}

type watcherImpl struct {
	ch     <-chan logLine
	cancel context.CancelFunc
}

func (w *watcherImpl) watch() <-chan logLine {
	return w.ch
}

func (w *watcherImpl) close() {
	w.cancel()
}

func watch(
	ctx context.Context,
	cmds [][]string,
	cacheExpiration time.Duration,
	cachePurgeInterval time.Duration,
) (<-chan logLine, error) {
	// initial 'tail' command may return >1k lines, buffer 3k to minimize the event loss
	ch := make(chan logLine, 3000)

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

func read(ctx context.Context, p process.Process, cacheExpiration time.Duration, cachePurgeInterval time.Duration, ch chan<- logLine) {
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

			parsed := parseLogLine(line)
			if occurrences := deduper.addCache(parsed); occurrences > 1 {
				log.Logger.Warnw("skipping duplicate log line", "occurrences", occurrences, "timestamp", parsed.ts, "line", parsed.content)
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
		case ch <- logLine{
			ts:  time.Now().UTC(),
			err: fmt.Errorf("reading output failed: %v", err),
		}:
			log.Logger.Warnw("failed to read dmesg output", "err", err)
		}
	}
}
