package kmsg

import (
	"bufio"
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

	t.Run("same content different timestamps should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		content := "test content"
		msg1 := Message{
			Timestamp: metav1.Time{Time: time.Now().UTC()},
			Message:   content,
		}
		msg2 := Message{
			Timestamp: metav1.Time{Time: time.Now().UTC().Add(1 * time.Second)},
			Message:   content,
		}
		assert.Equal(t, 1, d.addCache(msg1), "first timestamp first occurrence")
		assert.Equal(t, 1, d.addCache(msg2), "second timestamp first occurrence")
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

		// The first 7 messages are duplicates (same message, different timestamps)
		// The 8th message is different

		// First message should return 1 (first occurrence)
		assert.Equal(t, 1, d.addCache(messages[0]), "first message occurrence")

		// Second message should return 2 (second occurrence) - and so on
		assert.Equal(t, 2, d.addCache(messages[1]), "second message occurrence")
		assert.Equal(t, 3, d.addCache(messages[2]), "third message occurrence")
		assert.Equal(t, 4, d.addCache(messages[3]), "fourth message occurrence")
		assert.Equal(t, 5, d.addCache(messages[4]), "fifth message occurrence")
		assert.Equal(t, 6, d.addCache(messages[5]), "sixth message occurrence")
		assert.Equal(t, 7, d.addCache(messages[6]), "seventh message occurrence")

		// Last message is different, should return 1 (first occurrence)
		assert.Equal(t, 1, d.addCache(messages[7]), "different message occurrence")
	})

	t.Run("should handle same message with microsecond timestamp differences", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)

		// In the test data, messages have timestamps like 227996636, 227996637, etc.
		// These tiny differences might affect caching if using raw timestamps

		// Verify that messages with nearly identical timestamps but identical content
		// are still considered duplicates
		for i := 0; i < 7; i++ {
			expected := i + 1 // Expected occurrence count (1-based)
			assert.Equal(t, expected, d.addCache(messages[i]),
				"message %d with timestamp %d should be occurrence %d",
				i, messages[i].Timestamp.Unix(), expected)
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

		// But they should have different timestamps and different cache keys
		for i := 0; i < 7; i++ {
			for j := i + 1; j < 7; j++ {
				// Messages should have same content but different timestamps
				assert.Equal(t, messages[i].Message, messages[j].Message)
				assert.NotEqual(t, messages[i].Timestamp, messages[j].Timestamp)

				// By default, cacheKey uses Unix seconds, so these will be same in test data
				// Let's test the actual implementation
				if messages[i].Timestamp.Unix() == messages[j].Timestamp.Unix() {
					assert.Equal(t, messages[i].cacheKey(), messages[j].cacheKey(),
						"messages with same second-level timestamp should have same cache key")
				} else {
					assert.NotEqual(t, messages[i].cacheKey(), messages[j].cacheKey(),
						"messages with different second-level timestamps should have different cache keys")
				}
			}
		}
	})
}
