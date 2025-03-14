package dmesg

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

type LogLineProcessor struct {
	ctx          context.Context
	dmesgWatcher Watcher
	matchFunc    MatchFunc
	eventBucket  eventstore.Bucket
}

type MatchFunc func(line string) (eventName string, message string)

// NewLogLineProcessor creates a new log line processor where it streams logs from the dmesg watcher, uses the match function,
// and inserts the events into the events store.
func NewLogLineProcessor(ctx context.Context, matchFunc MatchFunc, eventBucket eventstore.Bucket) (*LogLineProcessor, error) {
	return newLogLineProcessor(ctx, nil, matchFunc, eventBucket)
}

// If the dmesg watcher is not provided, it will create a default one.
func newLogLineProcessor(ctx context.Context, dmesgWatcher Watcher, matchFunc MatchFunc, eventBucket eventstore.Bucket) (*LogLineProcessor, error) {
	if dmesgWatcher == nil {
		var err error
		dmesgWatcher, err = NewWatcher()
		if err != nil {
			return nil, err
		}
	}

	w := &LogLineProcessor{
		ctx:          ctx,
		dmesgWatcher: dmesgWatcher,
		matchFunc:    matchFunc,
		eventBucket:  eventBucket,
	}
	go w.watch()
	return w, nil
}

const EventKeyLogLine = "log_line"

func (w *LogLineProcessor) watch() {
	ch := w.dmesgWatcher.Watch()
	for {
		select {
		case <-w.ctx.Done():
			return
		case line, open := <-ch:
			if !open {
				return
			}
			if line.IsEmpty() {
				continue
			}

			ev := components.Event{
				Time: metav1.Time{Time: line.Timestamp.UTC()},
				Type: common.EventTypeWarning,
				ExtraInfo: map[string]string{
					EventKeyLogLine: line.Content,
				},
			}

			ev.Name, ev.Message = w.matchFunc(line.Content)
			if ev.Name == "" {
				continue
			}

			// lookup to prevent duplicate event insertions
			cctx, ccancel := context.WithTimeout(w.ctx, 15*time.Second)
			found, err := w.eventBucket.Find(
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
			cctx, ccancel = context.WithTimeout(w.ctx, 15*time.Second)
			err = w.eventBucket.Insert(cctx, ev)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to insert event", "error", err)
			} else {
				log.Logger.Infow("successfully inserted event", "event", ev.Name)
			}
		}
	}
}

// Get returns the event in the descending order of timestamp (latest event first).
func (w *LogLineProcessor) Get(ctx context.Context, since time.Time) ([]components.Event, error) {
	return w.eventBucket.Get(ctx, since)
}

func (w *LogLineProcessor) Close() {
	w.dmesgWatcher.Close()
}
