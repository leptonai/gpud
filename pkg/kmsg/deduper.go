package kmsg

import (
	"fmt"
	"time"

	cache "github.com/patrickmn/go-cache"

	"github.com/leptonai/gpud/pkg/eventstore"
)

const (
	// round down to the nearest minute
	defaultCacheKeyTruncateSeconds = 60
	defaultCacheExpiration         = 15 * time.Minute
	defaultCachePurgeInterval      = 30 * time.Minute
)

type Op struct {
	cacheKeyTruncateSeconds int
	disableDedup            bool
	eventDedupWindowFunc    EventDedupWindowFunc
}

type OpOption func(*Op)

type EventDedupWindowFunc func(eventstore.Event) (time.Duration, bool)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
}

func WithCacheKeyTruncateSeconds(seconds int) OpOption {
	return func(op *Op) {
		if seconds > 0 {
			op.cacheKeyTruncateSeconds = seconds
		}
	}
}

func withDisableDedup() OpOption {
	return func(op *Op) {
		op.disableDedup = true
	}
}

// WithEventDedupWindowFunc applies an event-specific dedup window in the syncer.
// When the callback returns a positive window, pkg/kmsg deduplicates matching
// events against recent persisted events inside that time window instead of
// using the generic parsed-message cache and infinite exact-match lookup.
func WithEventDedupWindowFunc(fn EventDedupWindowFunc) OpOption {
	return func(op *Op) {
		op.eventDedupWindowFunc = fn
	}
}

func (m Message) cacheKey() string {
	return m.cacheKeyWithTruncateSeconds(defaultCacheKeyTruncateSeconds)
}

func (m Message) cacheKeyWithTruncateSeconds(truncateSeconds int) string {
	if truncateSeconds <= 0 {
		truncateSeconds = defaultCacheKeyTruncateSeconds
	}

	unixSeconds := m.Timestamp.Unix()

	// round down to the nearest minute (or configured window)
	truncated := unixSeconds - (unixSeconds % int64(truncateSeconds))

	return fmt.Sprintf("%d-%s", truncated, m.Message)
}

// caches the log lines and its frequencies
type deduper struct {
	cache                   *cache.Cache
	cacheKeyTruncateSeconds int
}

func newDeduper(cacheExpiration time.Duration, cachePurgeInterval time.Duration, opts ...OpOption) *deduper {
	op := &Op{
		cacheKeyTruncateSeconds: defaultCacheKeyTruncateSeconds,
	}
	op.applyOpts(opts)
	if op.disableDedup {
		return nil
	}
	if op.cacheKeyTruncateSeconds <= 0 {
		op.cacheKeyTruncateSeconds = defaultCacheKeyTruncateSeconds
	}

	return &deduper{
		cache:                   cache.New(cacheExpiration, cachePurgeInterval),
		cacheKeyTruncateSeconds: op.cacheKeyTruncateSeconds,
	}
}

// addCache returns the current count of occurrences of the log line, found in the cache.
// Returns 1 if the log line was not in the cache thus first occurrence.
// Returns 2 if the log line was in the cache once before, thus second occurrence.
func (d *deduper) addCache(m Message) int {
	return d.addCacheWithWindow(m, d.cacheKeyTruncateSeconds, cache.DefaultExpiration)
}

// addCacheWithWindow is like addCache but uses a custom truncation window and
// per-item cache TTL. This allows the caller to use a different dedup granularity
// for specific event types (e.g. 24 hours for ACCESS_REG) while sharing the same
// underlying cache instance.
func (d *deduper) addCacheWithWindow(m Message, truncateSeconds int, expiration time.Duration) int {
	k := m.cacheKeyWithTruncateSeconds(truncateSeconds)

	var freq int
	cur, found := d.cache.Get(k)
	if !found {
		freq = 1
	} else {
		v, _ := cur.(int)
		freq = v + 1
	}

	d.cache.Set(k, freq, expiration)
	return freq
}
