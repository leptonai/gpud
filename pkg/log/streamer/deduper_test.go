package streamer

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogLineCacheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		line1   LogLine
		line2   LogLine
		sameKey bool
	}{
		{
			name: "same timestamp and content",
			line1: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				Content: "test message",
			},
			line2: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				Content: "test message",
			},
			sameKey: true,
		},
		{
			name: "different timestamp different minute",
			line1: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				Content: "test message",
			},
			line2: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 1, 0, 0, time.UTC),
				Content: "test message",
			},
			sameKey: false,
		},
		{
			name: "different content",
			line1: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				Content: "test message 1",
			},
			line2: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				Content: "test message 2",
			},
			sameKey: false,
		},
		{
			name: "same minute different seconds",
			line1: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				Content: "test message",
			},
			line2: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 0, 30, 0, time.UTC),
				Content: "test message",
			},
			sameKey: true, // They have the same minute boundary
		},
		{
			name: "different minute boundary",
			line1: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 0, 59, 0, time.UTC),
				Content: "test message",
			},
			line2: LogLine{
				Time:    time.Date(2025, time.February, 25, 10, 1, 0, 0, time.UTC),
				Content: "test message",
			},
			sameKey: false, // Different minute boundaries
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1 := tt.line1.cacheKey()
			key2 := tt.line2.cacheKey()

			if tt.sameKey {
				assert.Equal(t, key1, key2, "keys should be the same")
			} else {
				assert.NotEqual(t, key1, key2, "keys should be different")
			}
		})
	}
}

func TestDeduper(t *testing.T) {
	t.Parallel()

	t.Run("new deduper initialization", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		assert.NotNil(t, d)
		assert.NotNil(t, d.cache)
	})

	t.Run("addCache first occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		line := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			Content: "test message",
		}
		count := d.addCache(line)
		assert.Equal(t, 1, count, "first occurrence should return 1")
	})

	t.Run("addCache second occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		line := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			Content: "test message",
		}
		d.addCache(line)
		count := d.addCache(line)
		assert.Equal(t, 2, count, "second occurrence should return 2")
	})

	t.Run("addCache multiple occurrences", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		line := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			Content: "test message",
		}

		for i := 1; i <= 5; i++ {
			count := d.addCache(line)
			assert.Equal(t, i, count, "occurrence %d should return %d", i, i)
		}
	})

	t.Run("different content should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		ts := time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC)

		line1 := LogLine{Time: ts, Content: "message 1"}
		line2 := LogLine{Time: ts, Content: "message 2"}

		assert.Equal(t, 1, d.addCache(line1), "first line first occurrence")
		assert.Equal(t, 1, d.addCache(line2), "second line first occurrence")
		assert.Equal(t, 2, d.addCache(line1), "first line second occurrence")
		assert.Equal(t, 2, d.addCache(line2), "second line second occurrence")
	})

	t.Run("different timestamps should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		content := "same message"

		line1 := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			Content: content,
		}
		line2 := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 1, 0, 0, time.UTC),
			Content: content,
		}

		assert.Equal(t, 1, d.addCache(line1), "first timestamp first occurrence")
		assert.Equal(t, 1, d.addCache(line2), "second timestamp first occurrence")
		assert.Equal(t, 2, d.addCache(line1), "first timestamp second occurrence")
	})

	t.Run("same minute different seconds should share count", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		content := "same message"

		line1 := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			Content: content,
		}
		line2 := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 0, 30, 0, time.UTC),
			Content: content,
		}

		assert.Equal(t, 1, d.addCache(line1), "first timestamp first occurrence")
		assert.Equal(t, 2, d.addCache(line2), "same minute should increment count")
	})

	t.Run("expiration resets count", func(t *testing.T) {
		shortExpiration := 10 * time.Millisecond
		d := newDeduper(shortExpiration, 20*time.Millisecond)

		line := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			Content: "test message",
		}

		assert.Equal(t, 1, d.addCache(line), "first occurrence")
		time.Sleep(shortExpiration * 2)
		assert.Equal(t, 1, d.addCache(line), "should be first occurrence again after expiration")
	})

	t.Run("log lines with errors", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		line := LogLine{
			Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			Content: "test message",
			Error:   assert.AnError,
		}
		count := d.addCache(line)
		assert.Equal(t, 1, count, "log lines with errors should still be cached")
	})
}

func TestCacheKeyFormat(t *testing.T) {
	t.Parallel()

	line := LogLine{
		Time:    time.Date(2025, time.February, 25, 10, 5, 30, 0, time.UTC),
		Content: "test message",
	}

	key := line.cacheKey()

	// The key should be in format: "truncatedUnixSeconds-content"
	// Time 2025-02-25 10:05:30 UTC truncated to minute boundary (10:05:00)
	expectedUnix := time.Date(2025, time.February, 25, 10, 5, 0, 0, time.UTC).Unix()
	expectedUnix = expectedUnix - (expectedUnix % defaultCacheKeyTruncateSeconds)
	expectedKey := fmt.Sprintf("%d-%s", expectedUnix, "test message")

	assert.Equal(t, expectedKey, key, "cache key format should match expected pattern")
}

func TestDeduperConcurrency(t *testing.T) {
	d := newDeduper(5*time.Minute, 10*time.Minute)
	line := LogLine{
		Time:    time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
		Content: "concurrent test",
	}

	// Run multiple goroutines adding the same line
	var wg sync.WaitGroup
	results := make([]int, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = d.addCache(line)
		}(i)
	}
	wg.Wait()

	// The cache should handle concurrent access properly
	// We should see counts between 1 and 10
	for _, count := range results {
		assert.GreaterOrEqual(t, count, 1, "count should be at least 1")
		assert.LessOrEqual(t, count, 10, "count should be at most 10")
	}

	// Verify the cache is working - adding the same line again should increment
	finalCount := d.addCache(line)
	assert.Greater(t, finalCount, 1, "final count should be greater than 1")
}
