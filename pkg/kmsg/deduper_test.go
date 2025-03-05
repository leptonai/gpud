package kmsg

import (
	"testing"
	"time"

	"github.com/euank/go-kmsg-parser/v3/kmsgparser"
	"github.com/stretchr/testify/assert"
)

func TestDeduper(t *testing.T) {
	t.Run("new deduper should create cache with correct expiration", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		assert.NotNil(t, d)
		assert.NotNil(t, d.cache)
	})

	t.Run("should return 1 for first occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		logLine := kmsgparser.Message{
			Timestamp: time.Now().UTC(),
			Message:   "test content",
		}
		assert.Equal(t, 1, d.addCache(logLine), "first occurrence should return 1")
	})

	t.Run("should return 2 for second occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		logLine := kmsgparser.Message{
			Timestamp: time.Now().UTC(),
			Message:   "test content",
		}
		d.addCache(logLine)
		assert.Equal(t, 2, d.addCache(logLine), "second occurrence should return 2")
	})

	t.Run("different content should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		now := time.Now().UTC()
		logLine1 := kmsgparser.Message{
			Timestamp: now,
			Message:   "test content 1",
		}
		logLine2 := kmsgparser.Message{
			Timestamp: now,
			Message:   "test content 2",
		}
		assert.Equal(t, 1, d.addCache(logLine1), "first line first occurrence")
		assert.Equal(t, 1, d.addCache(logLine2), "second line first occurrence")
		assert.Equal(t, 2, d.addCache(logLine1), "first line second occurrence")
	})

	t.Run("same content different timestamps should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		content := "test content"
		logLine1 := kmsgparser.Message{
			Timestamp: time.Now().UTC(),
			Message:   content,
		}
		logLine2 := kmsgparser.Message{
			Timestamp: time.Now().UTC().Add(1 * time.Second),
			Message:   content,
		}
		assert.Equal(t, 1, d.addCache(logLine1), "first timestamp first occurrence")
		assert.Equal(t, 1, d.addCache(logLine2), "second timestamp first occurrence")
	})

	t.Run("count should reset after expiration", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		logLine := kmsgparser.Message{
			Timestamp: time.Now().UTC(),
			Message:   "test content",
		}
		assert.Equal(t, 1, d.addCache(logLine), "first occurrence")

		// Force cache expiration by setting a very short expiration time
		d.cache.Set(cacheKey(logLine), 1, 1*time.Millisecond)
		time.Sleep(2 * time.Millisecond)
		assert.Equal(t, 1, d.addCache(logLine), "should be first occurrence again after expiration")
	})

	t.Run("should respect provided expiration time", func(t *testing.T) {
		shortExpiration := 10 * time.Millisecond
		d := newDeduper(shortExpiration, 20*time.Millisecond)
		logLine := kmsgparser.Message{
			Timestamp: time.Now().UTC(),
			Message:   "test content",
		}
		assert.Equal(t, 1, d.addCache(logLine), "first occurrence")

		// Wait for expiration
		time.Sleep(2 * shortExpiration)
		assert.Equal(t, 1, d.addCache(logLine), "should be first occurrence again after expiration")
	})
}

func TestLogLineCacheKey(t *testing.T) {
	t.Run("same log lines should have same cache key", func(t *testing.T) {
		now := time.Now().UTC()
		line1 := kmsgparser.Message{
			Timestamp: now,
			Message:   "test content",
		}
		line2 := kmsgparser.Message{
			Timestamp: now,
			Message:   "test content",
		}
		assert.Equal(t, cacheKey(line1), cacheKey(line2))
	})

	t.Run("different content should have different cache keys", func(t *testing.T) {
		now := time.Now().UTC()
		line1 := kmsgparser.Message{
			Timestamp: now,
			Message:   "test content 1",
		}
		line2 := kmsgparser.Message{
			Timestamp: now,
			Message:   "test content 2",
		}
		assert.NotEqual(t, cacheKey(line1), cacheKey(line2))
	})

	t.Run("different timestamps should have different cache keys", func(t *testing.T) {
		content := "test content"
		line1 := kmsgparser.Message{
			Timestamp: time.Now().UTC(),
			Message:   content,
		}
		line2 := kmsgparser.Message{
			Timestamp: time.Now().UTC().Add(1 * time.Second),
			Message:   content,
		}
		assert.NotEqual(t, cacheKey(line1), cacheKey(line2))
	})

	t.Run("priority should not affect cache key", func(t *testing.T) {
		now := time.Now().UTC()
		content := "test content"
		line1 := kmsgparser.Message{
			Timestamp: now,
			Priority:  1,
			Message:   content,
		}
		line2 := kmsgparser.Message{
			Timestamp: now,
			Priority:  2,
			Message:   content,
		}
		assert.Equal(t, cacheKey(line1), cacheKey(line2))
	})
}
