package kmsg

import (
	"fmt"
	"time"

	cache "github.com/patrickmn/go-cache"
)

const (
	// round down to the nearest minute
	defaultCacheKeyTruncateSeconds = 60
	defaultCacheExpiration         = 15 * time.Minute
	defaultCachePurgeInterval      = 30 * time.Minute
)

type Op struct {
	cacheKeyTruncateSeconds int
}

type OpOption func(*Op)

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
	k := d.cacheKey(m)

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

func (d *deduper) cacheKey(m Message) string {
	return m.cacheKeyWithTruncateSeconds(d.cacheKeyTruncateSeconds)
}
