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

func (m Message) cacheKey() string {
	unixSeconds := m.Timestamp.Unix()

	// round down to the nearest minute
	truncated := unixSeconds - (unixSeconds % defaultCacheKeyTruncateSeconds)

	return fmt.Sprintf("%d-%s", truncated, m.Message)
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

// addCache returns the current count of occurrences of the log line, found in the cache.
// Returns 1 if the log line was not in the cache thus first occurrence.
// Returns 2 if the log line was in the cache once before, thus second occurrence.
func (d *deduper) addCache(m Message) int {
	k := m.cacheKey()

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
