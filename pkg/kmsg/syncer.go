package kmsg

import (
	"context"
	"time"

	cache "github.com/patrickmn/go-cache"

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
			// Events with a custom dedup window get a longer truncation bucket
			// and cache TTL so they are coalesced over the configured period.
			if w.parsedDeduper != nil {
				parsedMsg := Message{
					Timestamp: kmsg.Timestamp,
					Message:   name + "_" + message,
				}
				truncSec, expiration := w.dedupParams(event)
				if occurrences := w.parsedDeduper.addCacheWithWindow(parsedMsg, truncSec, expiration); occurrences > 1 {
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

// dedupParams returns the cache truncation seconds and per-item TTL to use for
// the given event. Events with a custom dedup window get a wider truncation
// bucket and a TTL matching the window. All other events use the deduper defaults.
func (w *Syncer) dedupParams(event eventstore.Event) (truncateSeconds int, expiration time.Duration) {
	if w.eventDedupWindowFunc != nil {
		if window, ok := w.eventDedupWindowFunc(event); ok && window > 0 {
			return int(window.Seconds()), window
		}
	}
	return w.parsedDeduper.cacheKeyTruncateSeconds, cache.DefaultExpiration
}

func (w *Syncer) Close() {
	_ = w.watcher.Close()
}
