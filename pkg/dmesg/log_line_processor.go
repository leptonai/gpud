package dmesg

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/log"
)

type LogLineProcessor struct {
	ctx          context.Context
	dmesgWatcher Watcher
	matchFunc    MatchFunc
	eventsStore  events_db.Store
}

type MatchFunc func(line string) (name string, message string)

func NewLogLineProcessor(ctx context.Context, dmesgWatcher Watcher, matchFunc MatchFunc, eventsStore events_db.Store) *LogLineProcessor {
	w := &LogLineProcessor{
		ctx:          ctx,
		dmesgWatcher: dmesgWatcher,
		matchFunc:    matchFunc,
		eventsStore:  eventsStore,
	}
	go w.watch()
	return w
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
			found, err := w.eventsStore.Find(
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
			err = w.eventsStore.Insert(cctx, ev)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to insert event", "error", err)
			} else {
				log.Logger.Infow("successfully inserted event", "event", ev.Name)
			}
		}
	}
}

// Returns the event in the descending order of timestamp (latest event first).
func (w *LogLineProcessor) Get(ctx context.Context, since time.Time) ([]components.Event, error) {
	return w.eventsStore.Get(ctx, since)
}

func (w *LogLineProcessor) Close() {
	w.dmesgWatcher.Close()
}
