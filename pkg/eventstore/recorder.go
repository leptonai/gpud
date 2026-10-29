package eventstore

import (
	"context"
	"fmt"
	"time"

	cache "github.com/patrickmn/go-cache"

	"github.com/leptonai/gpud/pkg/log"
)

// DedupPolicy is GPUd's policy for coalescing repeated component events.
// Window uses epoch-aligned buckets; EventWindow overrides selected events with
// a window starting at their first occurrence.
type DedupPolicy struct {
	Window      time.Duration
	EventWindow func(Event) (time.Duration, bool)
}

// Deduplicator tracks content in epoch-aligned time buckets.
type Deduplicator struct {
	cache  *cache.Cache
	window time.Duration
}

func NewDeduplicator(window time.Duration) *Deduplicator {
	if window <= 0 {
		window = time.Minute
	}
	return &Deduplicator{cache: cache.New(15*time.Minute, 30*time.Minute), window: window}
}

func (d *Deduplicator) Accept(at time.Time, content string) bool {
	key := fmt.Sprintf("%s-%s", at.Truncate(d.window).UTC().Format(time.RFC3339Nano), content)
	_, found := d.cache.Get(key)
	d.cache.Set(key, true, cache.DefaultExpiration)
	return !found
}

// Recorder applies event policy and the persistent exact-match lookup used across
// daemon restarts. It owns no sensor, parser, polling loop, or channel.
type Recorder struct {
	bucket      Bucket
	policy      DedupPolicy
	raw, parsed *Deduplicator
}

func NewRecorder(bucket Bucket, policy DedupPolicy) *Recorder {
	return &Recorder{bucket: bucket, policy: policy, raw: NewDeduplicator(policy.Window), parsed: NewDeduplicator(policy.Window)}
}

// AcceptRaw preserves content dedup before stateful component matching. Event
// overrides bypass it so the event's own window controls repeat handling.
func (r *Recorder) AcceptRaw(at time.Time, content string) bool {
	return r.policy.EventWindow != nil || r.raw.Accept(at, content)
}

func (r *Recorder) Record(ctx context.Context, event Event) error {
	key := event.Name + "_" + event.Message
	accepted := false
	if r.policy.EventWindow != nil {
		if window, ok := r.policy.EventWindow(event); ok && window > 0 {
			cacheKey := "window-" + key
			first, found := r.parsed.cache.Get(cacheKey)
			at, valid := first.(time.Time)
			if found && valid && event.Time.Before(at.Add(window)) {
				return nil
			}
			r.parsed.cache.Set(cacheKey, event.Time, window)
			accepted = true
		}
	}
	if !accepted && !r.parsed.Accept(event.Time, key) {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	existing, err := r.bucket.Find(lookupCtx, event)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to find event", "eventName", event.Name, "error", err)
	}
	if existing != nil {
		return nil
	}
	insertCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return r.bucket.Insert(insertCtx, event)
}
