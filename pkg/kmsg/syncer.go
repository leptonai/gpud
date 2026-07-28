package kmsg

import (
	"context"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

// Syncer syncs kernel message matched by MatchFunc to eventstore bucket
type Syncer struct {
	ctx                  context.Context
	watcher              Watcher
	matchFunc            MatchFunc
	eventBucket          eventstore.Bucket
	parsedDeduper        *deduper
	eventDedupWindowFunc EventDedupWindowFunc
}

type MatchFunc func(line string) (eventName string, message string)

func NewSyncer(ctx context.Context, matchFunc MatchFunc, eventBucket eventstore.Bucket, opts ...OpOption) (*Syncer, error) {
	return newSyncer(ctx, nil, matchFunc, eventBucket, opts...)
}

func newSyncer(ctx context.Context, watcher Watcher, matchFunc MatchFunc, eventBucket eventstore.Bucket, opts ...OpOption) (*Syncer, error) {
	op := &Op{
		cacheKeyTruncateSeconds: defaultCacheKeyTruncateSeconds,
	}
	op.applyOpts(opts)

	if watcher == nil {
		var err error
		if op.eventDedupWindowFunc != nil {
			// The syncer owns dedup policy only when event-level overrides are
			// configured. In that case, its watcher should stream raw kmsg lines
			// without pre-filtering so the event-specific window can fully control
			// the effective dedup behavior.
			watcher, err = NewWatcher(withDisableDedup())
		} else {
			watcher, err = NewWatcher(opts...)
		}
		if err != nil {
			return nil, err
		}
	}

	parsedDeduper := newDeduper(defaultCacheExpiration, defaultCachePurgeInterval, opts...)
	if parsedDeduper == nil && op.eventDedupWindowFunc != nil {
		// Event-specific dedup windows rely on the in-memory cache even when
		// generic dedup is disabled. Create a deduper with default settings.
		parsedDeduper = newDeduper(defaultCacheExpiration, defaultCachePurgeInterval)
	}

	w := &Syncer{
		ctx:                  ctx,
		watcher:              watcher,
		matchFunc:            matchFunc,
		eventBucket:          eventBucket,
		parsedDeduper:        parsedDeduper,
		eventDedupWindowFunc: op.eventDedupWindowFunc,
	}
	ch, err := w.watcher.Watch()
	if err != nil {
		return nil, err
	}
	go w.sync(ch)
	return w, nil
}

func (w *Syncer) sync(ch <-chan Message) {
	for {
		select {
		case <-w.ctx.Done():
			return
		case kmsg, ok := <-ch:
			if !ok {
				return
			}

			name, message := w.matchFunc(kmsg.Message)
			if name == "" {
				continue
			}

			event := eventstore.Event{
				Time:    kmsg.Timestamp.UTC(),
				Name:    name,
				Message: message,
				Type:    string(apiv1.EventTypeWarning),
			}

			// Deduplicate by parsed event name and message using the in-memory
			// cache. Raw kernel messages may contain varying strings (e.g., PIDs)
			// that the matcher normalizes, so we dedup on the parsed form.
			// Event-specific windows start at the first occurrence instead of
			// using the generic epoch-aligned cache buckets.
			if w.parsedDeduper != nil {
				parsedMsg := Message{
					Timestamp: kmsg.Timestamp,
					Message:   name + "_" + message,
				}
				var occurrences int
				if window, ok := w.eventDedupWindow(event); ok {
					occurrences = w.parsedDeduper.addCacheWithinWindow(parsedMsg, window)
				} else {
					occurrences = w.parsedDeduper.addCache(parsedMsg)
				}
				if occurrences > 1 {
					log.Logger.Debugw("skipping duplicate parsed kmsg message", "occurrences", occurrences, "eventName", name, "message", message)
					continue
				}
			}

			// Exact-match lookup to prevent duplicate event insertions across
			// process restarts (the in-memory cache is cold after a restart).
			cctx, ccancel := context.WithTimeout(w.ctx, 15*time.Second)
			sameEvent, err := w.eventBucket.Find(cctx, event)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to find event", "eventName", event.Name, "eventType", event.Type, "error", err)
			}
			if sameEvent != nil {
				continue
			}

			// insert event
			cctx, ccancel = context.WithTimeout(w.ctx, 15*time.Second)
			err = w.eventBucket.Insert(cctx, event)
			ccancel()
			if err != nil {
				log.Logger.Errorw("failed to insert event", "error", err)
			} else {
				log.Logger.Infow("successfully inserted event", "event", event.Name)
			}
		}
	}
}

func (w *Syncer) eventDedupWindow(event eventstore.Event) (time.Duration, bool) {
	if w.eventDedupWindowFunc == nil {
		return 0, false
	}
	window, ok := w.eventDedupWindowFunc(event)
	return window, ok && window > 0
}

func (w *Syncer) Close() {
	_ = w.watcher.Close()
}
