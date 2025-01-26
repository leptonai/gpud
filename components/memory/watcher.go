package memory

import (
	"context"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/common"
	events_db "github.com/leptonai/gpud/components/db"
	memory_dmesg "github.com/leptonai/gpud/components/memory/dmesg"
	"github.com/leptonai/gpud/log"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type watcher struct {
	ctx         context.Context
	eventsStore events_db.Store

	closeOnce    sync.Once
	dmesgWatcher pkg_dmesg.Watcher
}

func newWatcher(ctx context.Context, eventsStore events_db.Store) (*watcher, error) {
	dw, err := pkg_dmesg.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &watcher{
		ctx:          ctx,
		eventsStore:  eventsStore,
		dmesgWatcher: dw,
	}
	go w.watch()

	return w, nil
}

const EventKeyLogLine = "log_line"

func (w *watcher) watch() {
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

			ev.Name, ev.Message = memory_dmesg.Match(line.Content)
			if ev.Name == "" {
				continue
			}

			cctx, ccancel := context.WithTimeout(w.ctx, 15*time.Second)
			found, err := w.eventsStore.Find(cctx, components.Event{
				Time: ev.Time,
				Name: ev.Name,
				Type: ev.Type,
			})
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to find event", "eventName", ev.Name, "eventType", ev.Type, "error", err)
			}
			if found != nil {
				continue
			}

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

func (w *watcher) close() {
	w.closeOnce.Do(func() {
		w.dmesgWatcher.Close()
	})
}
