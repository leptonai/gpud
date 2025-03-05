package kmsg

import (
	"fmt"
	"time"

	"github.com/euank/go-kmsg-parser/v3/kmsgparser"
	cache "github.com/patrickmn/go-cache"
)

func cacheKey(m kmsgparser.Message) string {
	return fmt.Sprintf("%d-%s", m.Timestamp.Unix(), m.Message)
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
func (d *deduper) addCache(m kmsgparser.Message) int {
	k := cacheKey(m)

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
