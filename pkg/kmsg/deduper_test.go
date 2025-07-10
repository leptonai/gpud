package kmsg

import (
	"bufio"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeduper(t *testing.T) {
	t.Run("new deduper should create cache with correct expiration", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		assert.NotNil(t, d)
		assert.NotNil(t, d.cache)
	})

	t.Run("should return 1 for first occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		msg := Message{
			Timestamp: metav1.Time{Time: time.Now().UTC()},
			Message:   "test content",
		}
		assert.Equal(t, 1, d.addCache(msg), "first occurrence should return 1")
	})

	t.Run("should return 2 for second occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		msg := Message{
			Timestamp: metav1.Time{Time: time.Now().UTC()},
			Message:   "test content",
		}
		d.addCache(msg)
		assert.Equal(t, 2, d.addCache(msg), "second occurrence should return 2")
	})

	t.Run("different content should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		now := time.Now().UTC()
		msg1 := Message{
			Timestamp: metav1.Time{Time: now},
			Message:   "test content 1",
		}
		msg2 := Message{
			Timestamp: metav1.Time{Time: now},
			Message:   "test content 2",
		}
		assert.Equal(t, 1, d.addCache(msg1), "first line first occurrence")
		assert.Equal(t, 1, d.addCache(msg2), "second line first occurrence")
		assert.Equal(t, 2, d.addCache(msg1), "first line second occurrence")
	})

	t.Run("same content different timestamps within same minute should be treated as duplicates", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		content := "test content"
		baseTime := time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)
		msg1 := Message{
			Timestamp: metav1.Time{Time: baseTime},
			Message:   content,
		}
		msg2 := Message{
			Timestamp: metav1.Time{Time: baseTime.Add(30 * time.Second)},
			Message:   content,
		}
		assert.Equal(t, 1, d.addCache(msg1), "first timestamp first occurrence")
		assert.Equal(t, 2, d.addCache(msg2), "second timestamp within same minute should be second occurrence")

		// Different minute should have independent count
		msg3 := Message{
			Timestamp: metav1.Time{Time: baseTime.Add(61 * time.Second)},
			Message:   content,
		}
		assert.Equal(t, 1, d.addCache(msg3), "different minute should be first occurrence")
	})

	t.Run("count should reset after expiration", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		msg := Message{
			Timestamp: metav1.Time{Time: time.Now().UTC()},
			Message:   "test content",
		}
		assert.Equal(t, 1, d.addCache(msg), "first occurrence")

		// Force cache expiration by setting a very short expiration time
		d.cache.Set(msg.cacheKey(), 1, 1*time.Millisecond)
		time.Sleep(2 * time.Millisecond)
		assert.Equal(t, 1, d.addCache(msg), "should be first occurrence again after expiration")
	})

	t.Run("should respect provided expiration time", func(t *testing.T) {
		shortExpiration := 10 * time.Millisecond
		d := newDeduper(shortExpiration, 20*time.Millisecond)
		msg := Message{
			Timestamp: metav1.Time{Time: time.Now().UTC()},
			Message:   "test content",
		}
		assert.Equal(t, 1, d.addCache(msg), "first occurrence")

		// Wait for expiration
		time.Sleep(2 * shortExpiration)
		assert.Equal(t, 1, d.addCache(msg), "should be first occurrence again after expiration")
	})
}

func TestCacheKey(t *testing.T) {
	t.Run("should round down to nearest minute", func(t *testing.T) {
		// Test various timestamps within the same minute
		baseTime := time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)
		msg := Message{
			Message: "test message",
		}

		// All timestamps within the same minute should produce the same cache key
		expectedKey := fmt.Sprintf("%d-%s", baseTime.Unix()-(baseTime.Unix()%60), msg.Message)

		// Test at the start of the minute
		msg.Timestamp = metav1.Time{Time: baseTime}
		assert.Equal(t, expectedKey, msg.cacheKey())

		// Test at 15 seconds
		msg.Timestamp = metav1.Time{Time: baseTime.Add(15 * time.Second)}
		assert.Equal(t, expectedKey, msg.cacheKey())

		// Test at 30 seconds
		msg.Timestamp = metav1.Time{Time: baseTime.Add(30 * time.Second)}
		assert.Equal(t, expectedKey, msg.cacheKey())

		// Test at 59 seconds
		msg.Timestamp = metav1.Time{Time: baseTime.Add(59 * time.Second)}
		assert.Equal(t, expectedKey, msg.cacheKey())
	})

	t.Run("should produce different keys for different minutes", func(t *testing.T) {
		msg := Message{
			Message: "test message",
		}

		// Test timestamps in different minutes
		time1 := time.Date(2024, 1, 1, 12, 30, 45, 0, time.UTC)
		time2 := time.Date(2024, 1, 1, 12, 31, 15, 0, time.UTC)
		time3 := time.Date(2024, 1, 1, 12, 32, 0, 0, time.UTC)

		msg.Timestamp = metav1.Time{Time: time1}
		key1 := msg.cacheKey()

		msg.Timestamp = metav1.Time{Time: time2}
		key2 := msg.cacheKey()

		msg.Timestamp = metav1.Time{Time: time3}
		key3 := msg.cacheKey()

		// All keys should be different
		assert.NotEqual(t, key1, key2)
		assert.NotEqual(t, key2, key3)
		assert.NotEqual(t, key1, key3)
	})

	t.Run("should produce different keys for different messages in same minute", func(t *testing.T) {
		baseTime := time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)

		msg1 := Message{
			Timestamp: metav1.Time{Time: baseTime},
			Message:   "message 1",
		}
		msg2 := Message{
			Timestamp: metav1.Time{Time: baseTime.Add(30 * time.Second)},
			Message:   "message 2",
		}

		assert.NotEqual(t, msg1.cacheKey(), msg2.cacheKey())
	})

	t.Run("should handle edge cases correctly", func(t *testing.T) {
		msg := Message{
			Message: "test message",
		}

		// Test at exactly midnight
		midnight := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		msg.Timestamp = metav1.Time{Time: midnight}
		midnightKey := msg.cacheKey()
		expectedMidnightKey := fmt.Sprintf("%d-%s", midnight.Unix(), msg.Message)
		assert.Equal(t, expectedMidnightKey, midnightKey)

		// Test one second before midnight
		beforeMidnight := midnight.Add(-1 * time.Second)
		msg.Timestamp = metav1.Time{Time: beforeMidnight}
		beforeKey := msg.cacheKey()
		assert.NotEqual(t, midnightKey, beforeKey)

		// Test at the last second of a minute (59 seconds)
		lastSecond := time.Date(2024, 1, 1, 12, 30, 59, 0, time.UTC)
		msg.Timestamp = metav1.Time{Time: lastSecond}
		lastSecondKey := msg.cacheKey()

		// Test at the first second of next minute
		firstSecond := time.Date(2024, 1, 1, 12, 31, 0, 0, time.UTC)
		msg.Timestamp = metav1.Time{Time: firstSecond}
		firstSecondKey := msg.cacheKey()

		assert.NotEqual(t, lastSecondKey, firstSecondKey)
	})

	t.Run("verify defaultCacheKeyTruncateSeconds is 60", func(t *testing.T) {
		// Ensure the constant is set to 60 seconds (1 minute)
		assert.Equal(t, 60, defaultCacheKeyTruncateSeconds)
	})

	t.Run("deduper should treat messages within same minute as duplicates", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		baseTime := time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)
		content := "duplicate message"

		// Add message at start of minute
		msg1 := Message{
			Timestamp: metav1.Time{Time: baseTime},
			Message:   content,
		}
		assert.Equal(t, 1, d.addCache(msg1))

		// Add same message 30 seconds later (same minute)
		msg2 := Message{
			Timestamp: metav1.Time{Time: baseTime.Add(30 * time.Second)},
			Message:   content,
		}
		assert.Equal(t, 2, d.addCache(msg2))

		// Add same message 59 seconds later (still same minute)
		msg3 := Message{
			Timestamp: metav1.Time{Time: baseTime.Add(59 * time.Second)},
			Message:   content,
		}
		assert.Equal(t, 3, d.addCache(msg3))

		// Add same message in next minute (should start fresh count)
		msg4 := Message{
			Timestamp: metav1.Time{Time: baseTime.Add(60 * time.Second)},
			Message:   content,
		}
		assert.Equal(t, 1, d.addCache(msg4))
	})
}

func TestDeduperWithRealData(t *testing.T) {
	// Open the test data file
	testFile, err := os.Open("testdata/kmsg.2.peermem.log")
	require.NoError(t, err)
	defer testFile.Close()

	// Use a fixed boot time for deterministic testing
	bootTime := time.Unix(1000, 0)

	// Read all lines from the file
	var lines []string
	scanner := bufio.NewScanner(testFile)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	require.NoError(t, scanner.Err())
	require.Len(t, lines, 8) // Verify we have all 8 lines

	// Parse lines into Message objects
	var messages []Message
	for _, line := range lines {
		msg, err := parseLine(bootTime, line)
		require.NoError(t, err)
		messages = append(messages, *msg)
	}

	t.Run("should detect duplicate messages in test data", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)

		// The first 7 messages have the same content but different timestamps
		// With minute-based deduplication, they will be considered duplicates if within same minute
		// The 8th message is different

		// Track counts by cache key to handle minute-based deduplication
		keyCounts := make(map[string]int)

		// Process first 7 messages (same content)
		for i := 0; i < 7; i++ {
			key := messages[i].cacheKey()
			keyCounts[key]++
			expected := keyCounts[key]
			actual := d.addCache(messages[i])
			assert.Equal(t, expected, actual,
				"message %d (timestamp: %v) should have occurrence count %d",
				i, messages[i].Timestamp.Time, expected)
		}

		// Last message is different, should return 1 (first occurrence)
		assert.Equal(t, 1, d.addCache(messages[7]), "different message occurrence")
	})

	t.Run("should handle same message with microsecond timestamp differences", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)

		// In the test data, messages have timestamps like 227996636, 227996637, etc.
		// With the minute-based caching, messages within the same minute will be considered duplicates

		// First, let's check if all 7 messages are within the same minute
		minTime := messages[0].Timestamp.Unix()
		maxTime := messages[6].Timestamp.Unix()
		withinSameMinute := (maxTime - minTime) < 60

		if withinSameMinute {
			// If all within same minute, they should all be duplicates
			for i := 0; i < 7; i++ {
				expected := i + 1 // Expected occurrence count (1-based)
				assert.Equal(t, expected, d.addCache(messages[i]),
					"message %d with timestamp %d should be occurrence %d",
					i, messages[i].Timestamp.Unix(), expected)
			}
		} else {
			// If spanning multiple minutes, need to check each message individually
			counts := make(map[string]int)
			for i := 0; i < 7; i++ {
				key := messages[i].cacheKey()
				counts[key]++
				assert.Equal(t, counts[key], d.addCache(messages[i]),
					"message %d should have correct occurrence count based on cache key",
					i)
			}
		}
	})

	t.Run("should distinguish between error and test messages", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)

		// Message grouping test
		// First add 3 error messages
		errorCount := 3
		for i := 0; i < errorCount; i++ {
			count := d.addCache(messages[i])
			assert.Equal(t, i+1, count, "error message %d should have count %d", i, i+1)
		}

		// Then add the test message (last message)
		testMsg := messages[7]
		assert.Equal(t, 1, d.addCache(testMsg), "test message should be first occurrence")

		// Add another error message
		assert.Equal(t, errorCount+1, d.addCache(messages[3]),
			"next error message should be occurrence %d", errorCount+1)

		// Add test message again
		assert.Equal(t, 2, d.addCache(testMsg), "repeated test message should be second occurrence")
	})

	t.Run("should respect cache key format with real messages", func(t *testing.T) {
		// Verify that the cache key format works correctly with real data

		// All error messages should have the same message content
		errorMessage := "nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing"
		for i := 0; i < 7; i++ {
			assert.Equal(t, errorMessage, messages[i].Message)
		}

		// Test cache key behavior with minute-based rounding
		for i := 0; i < 7; i++ {
			for j := i + 1; j < 7; j++ {
				// Messages should have same content but different timestamps
				assert.Equal(t, messages[i].Message, messages[j].Message)
				assert.NotEqual(t, messages[i].Timestamp, messages[j].Timestamp)

				// With minute-based rounding, check if they're in the same minute
				minI := messages[i].Timestamp.Unix() - (messages[i].Timestamp.Unix() % 60)
				minJ := messages[j].Timestamp.Unix() - (messages[j].Timestamp.Unix() % 60)

				if minI == minJ {
					assert.Equal(t, messages[i].cacheKey(), messages[j].cacheKey(),
						"messages within same minute should have same cache key")
				} else {
					assert.NotEqual(t, messages[i].cacheKey(), messages[j].cacheKey(),
						"messages in different minutes should have different cache keys")
				}
			}
		}
	})
}
