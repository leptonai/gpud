package dmesg

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/process"
)

func TestDeduper(t *testing.T) {
	t.Run("new deduper should create cache with correct expiration", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		assert.NotNil(t, d)
		assert.NotNil(t, d.cache)
	})

	t.Run("should return 1 for first occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		logLine := LogLine{
			Timestamp: time.Now().UTC(),
			Content:   "test content",
		}
		assert.Equal(t, 1, d.addCache(logLine), "first occurrence should return 1")
	})

	t.Run("should return 2 for second occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		logLine := LogLine{
			Timestamp: time.Now().UTC(),
			Content:   "test content",
		}
		d.addCache(logLine)
		assert.Equal(t, 2, d.addCache(logLine), "second occurrence should return 2")
	})

	t.Run("different content should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		now := time.Now().UTC()
		logLine1 := LogLine{
			Timestamp: now,
			Content:   "test content 1",
		}
		logLine2 := LogLine{
			Timestamp: now,
			Content:   "test content 2",
		}
		assert.Equal(t, 1, d.addCache(logLine1), "first line first occurrence")
		assert.Equal(t, 1, d.addCache(logLine2), "second line first occurrence")
		assert.Equal(t, 2, d.addCache(logLine1), "first line second occurrence")
	})

	t.Run("same content different timestamps should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		content := "test content"
		logLine1 := LogLine{
			Timestamp: time.Now().UTC(),
			Content:   content,
		}
		logLine2 := LogLine{
			Timestamp: time.Now().UTC().Add(1 * time.Second),
			Content:   content,
		}
		assert.Equal(t, 1, d.addCache(logLine1), "first timestamp first occurrence")
		assert.Equal(t, 1, d.addCache(logLine2), "second timestamp first occurrence")
	})

	t.Run("count should reset after expiration", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		logLine := LogLine{
			Timestamp: time.Now().UTC(),
			Content:   "test content",
		}
		assert.Equal(t, 1, d.addCache(logLine), "first occurrence")

		// Force cache expiration by setting a very short expiration time
		d.cache.Set(logLine.cacheKey(), 1, 1*time.Millisecond)
		time.Sleep(2 * time.Millisecond)
		assert.Equal(t, 1, d.addCache(logLine), "should be first occurrence again after expiration")
	})

	t.Run("should respect provided expiration time", func(t *testing.T) {
		shortExpiration := 10 * time.Millisecond
		d := newDeduper(shortExpiration, 20*time.Millisecond)
		logLine := LogLine{
			Timestamp: time.Now().UTC(),
			Content:   "test content",
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
		line1 := LogLine{
			Timestamp: now,
			Content:   "test content",
		}
		line2 := LogLine{
			Timestamp: now,
			Content:   "test content",
		}
		assert.Equal(t, line1.cacheKey(), line2.cacheKey())
	})

	t.Run("different content should have different cache keys", func(t *testing.T) {
		now := time.Now().UTC()
		line1 := LogLine{
			Timestamp: now,
			Content:   "test content 1",
		}
		line2 := LogLine{
			Timestamp: now,
			Content:   "test content 2",
		}
		assert.NotEqual(t, line1.cacheKey(), line2.cacheKey())
	})

	t.Run("different timestamps should have different cache keys", func(t *testing.T) {
		content := "test content"
		line1 := LogLine{
			Timestamp: time.Now().UTC(),
			Content:   content,
		}
		line2 := LogLine{
			Timestamp: time.Now().UTC().Add(1 * time.Second),
			Content:   content,
		}
		assert.NotEqual(t, line1.cacheKey(), line2.cacheKey())
	})

	t.Run("facility and level should not affect cache key", func(t *testing.T) {
		now := time.Now().UTC()
		content := "test content"
		line1 := LogLine{
			Timestamp: now,
			Facility:  "kern",
			Level:     "info",
			Content:   content,
		}
		line2 := LogLine{
			Timestamp: now,
			Facility:  "user",
			Level:     "warn",
			Content:   content,
		}
		assert.Equal(t, line1.cacheKey(), line2.cacheKey())
	})
}

func TestDedupLogLines(t *testing.T) {
	t.Run("should deduplicate log lines with same second timestamp", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := make(chan LogLine, 1000)
		p, err := process.New(process.WithCommand("echo", "-e", "kern  :info  : 2025-01-21T04:41:44,100000+00:00 Test message\nkern  :info  : 2025-01-21T04:41:44,200000+00:00 Test message\nkern  :info  : 2025-01-21T04:41:44,300000+00:00 Test message"))
		if err != nil {
			t.Fatalf("failed to create process: %v", err)
		}
		if err := p.Start(ctx); err != nil {
			t.Fatalf("failed to start process: %v", err)
		}

		go read(ctx, p, DefaultCacheExpiration, DefaultCachePurgeInterval, ch)

		var lines []LogLine
		for line := range ch {
			lines = append(lines, line)
		}

		assert.Equal(t, 1, len(lines), "expected only one log line after deduplication")
		assert.Equal(t, "Test message", lines[0].Content)
	})

	t.Run("should not deduplicate log lines with different second timestamps", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := make(chan LogLine, 1000)
		p, err := process.New(process.WithCommand("echo", "-e", "kern  :info  : 2025-01-21T04:41:44,100000+00:00 Test message\nkern  :info  : 2025-01-21T04:41:45,200000+00:00 Test message\nkern  :info  : 2025-01-21T04:41:46,300000+00:00 Test message"))
		if err != nil {
			t.Fatalf("failed to create process: %v", err)
		}
		if err := p.Start(ctx); err != nil {
			t.Fatalf("failed to start process: %v", err)
		}
		defer func() {
			if err := p.Close(ctx); err != nil {
				t.Fatalf("failed to close process: %v", err)
			}
		}()

		go read(ctx, p, DefaultCacheExpiration, DefaultCachePurgeInterval, ch)

		var lines []LogLine
		for line := range ch {
			lines = append(lines, line)
		}

		assert.Equal(t, 3, len(lines), "expected three log lines with different second timestamps")
		for _, line := range lines {
			assert.Equal(t, "Test message", line.Content)
		}
	})

	t.Run("should deduplicate mixed log lines with same and different seconds", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		ch := make(chan LogLine, 1000)
		p, err := process.New(process.WithCommand("echo", "-e", `kern  :info  : 2025-01-21T04:41:44,100000+00:00 Test message
kern  :info  : 2025-01-21T04:41:44,200000+00:00 Test message
kern  :info  : 2025-01-21T04:41:45,100000+00:00 Test message
kern  :info  : 2025-01-21T04:41:45,200000+00:00 Test message
kern  :info  : 2025-01-21T04:41:46,100000+00:00 Different message`))
		if err != nil {
			t.Fatalf("failed to create process: %v", err)
		}
		if err := p.Start(ctx); err != nil {
			t.Fatalf("failed to start process: %v", err)
		}
		defer func() {
			if err := p.Close(ctx); err != nil {
				t.Fatalf("failed to close process: %v", err)
			}
		}()

		go read(ctx, p, DefaultCacheExpiration, DefaultCachePurgeInterval, ch)

		var lines []LogLine
		for line := range ch {
			lines = append(lines, line)
		}

		assert.Equal(t, 3, len(lines), "expected three log lines after deduplication")

		// Count unique messages
		messageCount := make(map[string]int)
		for _, line := range lines {
			messageCount[line.Content]++
		}
		assert.Equal(t, 2, len(messageCount), "expected two unique messages")
		assert.Equal(t, 2, messageCount["Test message"], "expected two 'Test message' entries (different seconds)")
		assert.Equal(t, 1, messageCount["Different message"], "expected one 'Different message' entry")
	})

	t.Run("should deduplicate same content across multiple seconds", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create a string with multiple log lines in the same second
		var logLines []string
		baseTime := time.Date(2025, 1, 21, 4, 41, 44, 0, time.UTC)
		for i := 0; i < 5; i++ {
			for ms := 0; ms < 3; ms++ {
				timestamp := baseTime.Add(time.Duration(i) * time.Second)
				logLines = append(logLines, fmt.Sprintf("kern  :info  : %s,%06d+00:00 Test message",
					timestamp.Format("2006-01-02T15:04:05"),
					ms*100000))
			}
		}

		ch := make(chan LogLine, 1000)
		p, err := process.New(process.WithCommand("echo", "-e", strings.Join(logLines, "\n")))
		if err != nil {
			t.Fatalf("failed to create process: %v", err)
		}
		if err := p.Start(ctx); err != nil {
			t.Fatalf("failed to start process: %v", err)
		}
		defer func() {
			if err := p.Close(ctx); err != nil {
				t.Fatalf("failed to close process: %v", err)
			}
		}()

		go read(ctx, p, DefaultCacheExpiration, DefaultCachePurgeInterval, ch)

		var lines []LogLine
		for line := range ch {
			lines = append(lines, line)
		}

		assert.Equal(t, 5, len(lines), "expected five log lines (one per second) after deduplication")

		// Verify timestamps are different
		seenTimestamps := make(map[int64]bool)
		for _, line := range lines {
			seenTimestamps[line.Timestamp.Unix()] = true
		}
		assert.Equal(t, 5, len(seenTimestamps), "expected five different timestamps")
	})

	t.Run("should not deduplicate same second logs with slightly different content", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create log lines with same second but slightly different content
		logLines := []string{
			"kern  :err   : 2025-02-10T16:28:06,502716+00:00 nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing",
			"kern  :err   : 2025-02-10T16:28:06,514050+00:00 nvidia-peermem nv_get_p2p_free_callback:128 ERROR detected invalid context, skipping further processing", // note the 127->128
			"kern  :err   : 2025-02-10T16:28:06,525389+00:00 nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing",
		}

		ch := make(chan LogLine, 1000)
		p, err := process.New(process.WithCommand("echo", "-e", strings.Join(logLines, "\n")))
		if err != nil {
			t.Fatalf("failed to create process: %v", err)
		}
		if err := p.Start(ctx); err != nil {
			t.Fatalf("failed to start process: %v", err)
		}
		defer func() {
			if err := p.Close(ctx); err != nil {
				t.Fatalf("failed to close process: %v", err)
			}
		}()

		go read(ctx, p, DefaultCacheExpiration, DefaultCachePurgeInterval, ch)

		var lines []LogLine
		for line := range ch {
			lines = append(lines, line)
		}

		// Should get 2 lines: one for "127" and one for "128"
		assert.Equal(t, 2, len(lines), "expected two log lines (one for each unique content)")

		// Count occurrences of each line number
		lineNumbers := make(map[string]bool)
		for _, line := range lines {
			if strings.Contains(line.Content, "callback:127") {
				lineNumbers["127"] = true
			}
			if strings.Contains(line.Content, "callback:128") {
				lineNumbers["128"] = true
			}
		}

		assert.Equal(t, 2, len(lineNumbers), "expected both line numbers (127 and 128) to be present")
		assert.True(t, lineNumbers["127"], "expected line with callback:127 to be present")
		assert.True(t, lineNumbers["128"], "expected line with callback:128 to be present")

		// Verify all lines are from the same second
		baseTimestamp := lines[0].Timestamp.Unix()
		for _, line := range lines {
			assert.Equal(t, baseTimestamp, line.Timestamp.Unix(), "all lines should have the same second timestamp")
		}
	})

	t.Run("should deduplicate log lines with same content but different trailing whitespaces", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create log lines with same content but different trailing whitespaces
		logLines := []string{
			"kern  :err   : 2025-02-10T16:28:06,502716+00:00 nvidia-peermem error message",
			"kern  :err   : 2025-02-10T16:28:06,514050+00:00 nvidia-peermem error message  ",   // two spaces at end
			"kern  :err   : 2025-02-10T16:28:06,525389+00:00 nvidia-peermem error message\t",   // tab at end
			"kern  :err   : 2025-02-10T16:28:06,535389+00:00 nvidia-peermem error message \t ", // mixed whitespace at end
		}

		ch := make(chan LogLine, 1000)
		p, err := process.New(process.WithCommand("echo", "-e", strings.Join(logLines, "\n")))
		if err != nil {
			t.Fatalf("failed to create process: %v", err)
		}
		if err := p.Start(ctx); err != nil {
			t.Fatalf("failed to start process: %v", err)
		}
		defer func() {
			if err := p.Close(ctx); err != nil {
				t.Fatalf("failed to close process: %v", err)
			}
		}()

		go read(ctx, p, DefaultCacheExpiration, DefaultCachePurgeInterval, ch)

		var lines []LogLine
		for line := range ch {
			lines = append(lines, line)
		}

		// Should get only 1 line since they're all the same content with different whitespace
		assert.Equal(t, 1, len(lines), "expected one log line after deduplication of whitespace variants")
		assert.Equal(t, "nvidia-peermem error message", strings.TrimSpace(lines[0].Content),
			"expected content to be trimmed and deduplicated")

		// Verify timestamp is from the expected second
		expectedTimestamp := time.Date(2025, 2, 10, 16, 28, 6, 0, time.UTC).Unix()
		assert.Equal(t, expectedTimestamp, lines[0].Timestamp.Unix(),
			"all lines should have been from the same second")
	})
}

func TestWatchPeerMemLogs(t *testing.T) {
	w, err := NewWatcherWithCommands([][]string{
		{"cat", "testdata/dmesg.peermem.log.0"},
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	var lines []LogLine
	for line := range w.Watch() {
		lines = append(lines, line)
	}

	// All log lines in peermem.log.0 are from the same second and have the same content,
	// so they should be deduplicated into a single entry
	assert.Equal(t, 2, len(lines), "expected only one log line after deduplication")

	expectedLine := LogLine{
		Timestamp: time.Date(2025, 2, 10, 16, 28, 6, 502716000, time.UTC),
		Facility:  "kern",
		Level:     "err",
		Content:   "nvidia-peermem nv_get_p2p_free_callback:127 ERROR detected invalid context, skipping further processing",
	}

	// Verify the content of the deduplicated line
	assert.Equal(t, expectedLine.Facility, lines[0].Facility)
	assert.Equal(t, expectedLine.Level, lines[0].Level)
	assert.Equal(t, expectedLine.Content, lines[0].Content)

	// Verify timestamp is from the same second
	assert.Equal(t, expectedLine.Timestamp.Unix(), lines[0].Timestamp.Unix())

	assert.Equal(t, "test", lines[1].Content)
}
