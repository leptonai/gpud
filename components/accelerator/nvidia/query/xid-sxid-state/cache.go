package xidsxidstate

import "github.com/coocood/freecache"

// e.g.,
// xid 13 (applications having illegal memory access issues) is triggered by the application,
// can flood dmesg with the same error message with similar timestamps
// need in-memory deduplication to avoid excessive db reads/writes
type EventDeduper interface {
	Get(event Event) bool
	Add(event Event) error
}

var _ EventDeduper = (*eventDeduper)(nil)

const (
	DefaultCacheSizeInBytes  = 100 * 1024 * 1024 // 100 MB
	DefaultCacheTTLInSeconds = 10 * 60           // 10 minutes
)

type eventDeduper struct {
	cache      *freecache.Cache
	ttlSeconds int
}

func NewEventDeduper(cacheSizeInBytes int, ttlInSeconds int) *eventDeduper {
	if cacheSizeInBytes == 0 {
		cacheSizeInBytes = DefaultCacheSizeInBytes
	}
	if ttlInSeconds == 0 {
		ttlInSeconds = DefaultCacheTTLInSeconds
	}
	return &eventDeduper{
		cache:      freecache.NewCache(cacheSizeInBytes),
		ttlSeconds: ttlInSeconds,
	}
}

func (e *eventDeduper) Get(event Event) bool {
	cacheEntryID := event.cacheEntryIDWithTruncatedMinute()
	_, err := e.cache.Get([]byte(cacheEntryID))
	return err == nil
}

func (e *eventDeduper) Add(event Event) error {
	cacheEntryID := event.cacheEntryIDWithTruncatedMinute()
	return e.cache.Set([]byte(cacheEntryID), []byte(""), e.ttlSeconds)
}
