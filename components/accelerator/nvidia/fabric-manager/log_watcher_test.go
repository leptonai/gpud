package fabricmanager

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestParseLogLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		expectedTime time.Time
		expectedCont string
		expectErr    bool
	}{
		{
			name:         "valid log line",
			input:        "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Received an inband message",
			expectedTime: time.Date(2025, time.February, 25, 13, 59, 45, 0, time.UTC),
			expectedCont: "[INFO] [tid 1803] Received an inband message",
			expectErr:    false,
		},
		{
			name:         "valid log line with error level",
			input:        "[Feb 27 2025 15:10:02] [ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0",
			expectedTime: time.Date(2025, time.February, 27, 15, 10, 2, 0, time.UTC),
			expectedCont: "[ERROR] [tid 12727] detected NVSwitch non-fatal error 12028 on fid 0",
			expectErr:    false,
		},
		{
			name:         "no timestamp",
			input:        "This is a line without a timestamp",
			expectedTime: time.Time{},
			expectedCont: "This is a line without a timestamp",
			expectErr:    true,
		},
		{
			name:         "invalid timestamp format",
			input:        "[2025-02-25 13:59:45] Some content",
			expectedTime: time.Time{},
			expectedCont: "[2025-02-25 13:59:45] Some content",
			expectErr:    true,
		},
		{
			name:         "empty line",
			input:        "",
			expectedTime: time.Time{},
			expectedCont: "",
			expectErr:    true,
		},
		{
			name:         "timestamp only",
			input:        "[Feb 25 2025 13:59:45]",
			expectedTime: time.Date(2025, time.February, 25, 13, 59, 45, 0, time.UTC),
			expectedCont: "[Feb 25 2025 13:59:45]",
			expectErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLogLine(tt.input)

			if tt.expectErr {
				assert.NotNil(t, result.err)
			} else {
				assert.Nil(t, result.err)
				assert.Equal(t, tt.expectedTime, result.ts, "timestamp should match")
				assert.Equal(t, tt.expectedCont, result.content, "content should match")
			}
		})
	}
}

func TestLogLineCacheKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		line1   logLine
		line2   logLine
		sameKey bool
	}{
		{
			name: "same timestamp and content",
			line1: logLine{
				ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				content: "test message",
			},
			line2: logLine{
				ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				content: "test message",
			},
			sameKey: true,
		},
		{
			name: "different timestamp",
			line1: logLine{
				ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				content: "test message",
			},
			line2: logLine{
				ts:      time.Date(2025, time.February, 25, 10, 0, 1, 0, time.UTC),
				content: "test message",
			},
			sameKey: false,
		},
		{
			name: "different content",
			line1: logLine{
				ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				content: "test message 1",
			},
			line2: logLine{
				ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				content: "test message 2",
			},
			sameKey: false,
		},
		{
			name: "same second different millisecond",
			line1: logLine{
				ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
				content: "test message",
			},
			line2: logLine{
				ts:      time.Date(2025, time.February, 25, 10, 0, 0, 500000000, time.UTC),
				content: "test message",
			},
			sameKey: true, // They have the same unix timestamp at second precision
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
		line := logLine{
			ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			content: "test message",
		}
		count := d.addCache(line)
		assert.Equal(t, 1, count, "first occurrence should return 1")
	})

	t.Run("addCache second occurrence", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		line := logLine{
			ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			content: "test message",
		}
		d.addCache(line)
		count := d.addCache(line)
		assert.Equal(t, 2, count, "second occurrence should return 2")
	})

	t.Run("different content should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		ts := time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC)

		line1 := logLine{ts: ts, content: "message 1"}
		line2 := logLine{ts: ts, content: "message 2"}

		assert.Equal(t, 1, d.addCache(line1), "first line first occurrence")
		assert.Equal(t, 1, d.addCache(line2), "second line first occurrence")
		assert.Equal(t, 2, d.addCache(line1), "first line second occurrence")
	})

	t.Run("different timestamps should have independent counts", func(t *testing.T) {
		d := newDeduper(5*time.Minute, 10*time.Minute)
		content := "same message"

		line1 := logLine{
			ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			content: content,
		}
		line2 := logLine{
			ts:      time.Date(2025, time.February, 25, 10, 0, 1, 0, time.UTC),
			content: content,
		}

		assert.Equal(t, 1, d.addCache(line1), "first timestamp first occurrence")
		assert.Equal(t, 1, d.addCache(line2), "second timestamp first occurrence")
		assert.Equal(t, 2, d.addCache(line1), "first timestamp second occurrence")
	})

	t.Run("expiration resets count", func(t *testing.T) {
		shortExpiration := 10 * time.Millisecond
		d := newDeduper(shortExpiration, 20*time.Millisecond)

		line := logLine{
			ts:      time.Date(2025, time.February, 25, 10, 0, 0, 0, time.UTC),
			content: "test message",
		}

		assert.Equal(t, 1, d.addCache(line), "first occurrence")
		time.Sleep(shortExpiration * 2)
		assert.Equal(t, 1, d.addCache(line), "should be first occurrence again after expiration")
	})
}

func TestWatcher(t *testing.T) {
	t.Run("new watcher with empty commands", func(t *testing.T) {
		w, err := newWatcher([][]string{})
		assert.Error(t, err)
		assert.Nil(t, w)
	})

	t.Run("new watcher with valid commands", func(t *testing.T) {
		w, err := newWatcher([][]string{{"echo", "test"}})
		assert.NoError(t, err)
		assert.NotNil(t, w)
		defer w.close()
	})

	t.Run("watch and close", func(t *testing.T) {
		w, err := newWatcher([][]string{{"echo", "test message"}})
		assert.NoError(t, err)
		assert.NotNil(t, w)

		ch := w.watch()
		assert.NotNil(t, ch)

		// Close should not panic
		w.close()
		w.close() // Second close should be safe
	})
}

func TestWatchFabricManagerLogs(t *testing.T) {
	testDataPath := "testdata/fabricmanager.log"

	t.Run("watch fabricmanager logs with test data", func(t *testing.T) {
		w, err := newWatcher([][]string{
			{"cat", testDataPath},
			{"sleep", "2"}, // Small delay to ensure all lines are read
		})
		require.NoError(t, err)
		require.NotNil(t, w)
		defer w.close()

		ch := w.watch()
		require.NotNil(t, ch)

		var lines []logLine
		for line := range ch {
			lines = append(lines, line)
		}

		// Check if we got log lines
		assert.NotEmpty(t, lines, "should have parsed log lines")

		// Find the error line about NVSwitch non-fatal error
		var foundErrorLine bool
		for _, line := range lines {
			if strings.Contains(line.content, "[ERROR] [tid 12727] detected NVSwitch non-fatal error") {
				foundErrorLine = true
				// Verify timestamp
				expectedTime := time.Date(2025, time.February, 27, 15, 10, 2, 0, time.UTC)
				assert.Equal(t, expectedTime, line.ts, "error log should have correct timestamp")
				break
			}
		}
		assert.True(t, foundErrorLine, "should find NVSwitch error log line")
	})

	t.Run("watch with deduplication", func(t *testing.T) {
		// Create duplicate lines for testing
		ctx := t.Context()

		// Create a process that outputs duplicate lines with same timestamp
		p, err := process.New(
			process.WithCommand("echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Duplicate message 1"),
			process.WithCommand("echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Duplicate message 1"),
			process.WithCommand("echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Duplicate message 1"),
			process.WithCommand("echo", "[Feb 25 2025 13:59:46] [INFO] [tid 1803] Different second"),
			process.WithCommand("echo", "[Feb 25 2025 13:59:45] [INFO] [tid 1803] Different message"),
			process.WithCommand("sleep", "1"),
			process.WithRunAsBashScript(),
			process.WithRunBashInline(),
		)
		require.NoError(t, err)

		// Start collecting results
		ch := make(chan logLine, 100)
		done := make(chan struct{})
		var lines []logLine

		go func() {
			defer close(done)
			for line := range ch {
				lines = append(lines, line)
			}
		}()

		require.NoError(t, p.Start(ctx))
		defer func() {
			if err := p.Close(ctx); err != nil {
				t.Logf("Failed to close process: %v", err)
			}
		}()

		read(ctx, p, defaultCacheExpiration, defaultCachePurgeInterval, ch)

		// Wait for lines to be collected
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for lines")
		}

		// We should have 3 unique lines:
		// 1. Duplicate message 1 (only once due to deduplication)
		// 2. Different second
		// 3. Different message
		assert.Equal(t, 3, len(lines), "expected 3 unique log lines after deduplication")

		// Verify messages
		messages := make(map[string]bool)
		for _, line := range lines {
			messages[line.content] = true
		}

		assert.True(t, messages["[INFO] [tid 1803] Duplicate message 1"], "should have the first duplicate line")
		assert.True(t, messages["[INFO] [tid 1803] Different second"], "should have the different second line")
		assert.True(t, messages["[INFO] [tid 1803] Different message"], "should have the different message line")
	})
}

func TestErrorHandling(t *testing.T) {
	t.Run("parse log line with invalid timestamp", func(t *testing.T) {
		line := "[Invalid date format] [INFO] Test"
		result := parseLogLine(line)
		assert.NotNil(t, result.err)
		assert.Equal(t, line, result.content)
	})

	t.Run("parse log line with missing timestamp", func(t *testing.T) {
		line := "No timestamp here"
		result := parseLogLine(line)
		assert.NotNil(t, result.err)
		assert.Equal(t, line, result.content)
	})
}

func TestMultipleWatchers(t *testing.T) {
	t.Run("create multiple watchers and close them", func(t *testing.T) {
		watchers := make([]watcher, 3)
		for i := range 3 {
			w, err := newWatcher([][]string{{"echo", fmt.Sprintf("test %d", i)}})
			require.NoError(t, err)
			watchers[i] = w
		}

		// Close all watchers
		for _, w := range watchers {
			w.close()
		}
	})
}

func TestParallelLogWatching(t *testing.T) {
	t.Run("watch two sources in parallel", func(t *testing.T) {
		// Construct a command similar to the default one but with echos and sleeps to verify parallelism
		// Source 1: prints immediately, then sleeps
		// Source 2: sleeps then prints
		// Both backgrounded and waited
		// Uses single string command style as in defaultWatchCommands
		cmd := "echo '[Feb 25 2025 10:00:00] [INFO] Source1' && sleep 2 & " +
			"sleep 1 && echo '[Feb 25 2025 10:00:01] [INFO] Source2' & " +
			"wait"

		w, err := newWatcher([][]string{
			{cmd},
		})
		require.NoError(t, err)
		defer w.close()

		ch := w.watch()

		var lines []logLine
		timeout := time.After(5 * time.Second)

		// Collect lines
	loop:
		for {
			select {
			case line, ok := <-ch:
				if !ok {
					break loop
				}
				lines = append(lines, line)
				if len(lines) >= 2 {
					break loop
				}
			case <-timeout:
				t.Fatal("timeout waiting for log lines")
			}
		}

		assert.Equal(t, 2, len(lines), "should get output from both sources")

		// Verify we got both messages
		contents := make(map[string]bool)
		for _, l := range lines {
			contents[l.content] = true
		}
		assert.True(t, contents["[INFO] Source1"], "should have source 1 output")
		assert.True(t, contents["[INFO] Source2"], "should have source 2 output")
	})
}

func TestEventDeduper(t *testing.T) {
	t.Parallel()

	t.Run("first occurrence returns 1", func(t *testing.T) {
		d := newEventDeduper(15 * time.Minute)
		assert.Equal(t, 1, d.addCache("event_name", "event message"))
	})

	t.Run("second occurrence returns 2", func(t *testing.T) {
		d := newEventDeduper(15 * time.Minute)
		d.addCache("event_name", "event message")
		assert.Equal(t, 2, d.addCache("event_name", "event message"))
	})

	t.Run("different event names have independent counts", func(t *testing.T) {
		d := newEventDeduper(15 * time.Minute)
		assert.Equal(t, 1, d.addCache("event_a", "same message"))
		assert.Equal(t, 1, d.addCache("event_b", "same message"))
		assert.Equal(t, 2, d.addCache("event_a", "same message"))
	})

	t.Run("different messages have independent counts", func(t *testing.T) {
		d := newEventDeduper(15 * time.Minute)
		assert.Equal(t, 1, d.addCache("event_name", "message 1"))
		assert.Equal(t, 1, d.addCache("event_name", "message 2"))
		assert.Equal(t, 2, d.addCache("event_name", "message 1"))
	})

	t.Run("expiration resets count", func(t *testing.T) {
		d := newEventDeduper(10 * time.Millisecond)
		assert.Equal(t, 1, d.addCache("event_name", "message"))
		assert.Equal(t, 2, d.addCache("event_name", "message"))
		time.Sleep(30 * time.Millisecond)
		assert.Equal(t, 1, d.addCache("event_name", "message"), "should be first occurrence again after expiration")
	})
}

func TestEventDedupDefaultWindow(t *testing.T) {
	t.Parallel()

	// Verify the default event dedup window is 15 minutes.
	assert.Equal(t, 15*time.Minute, defaultEventDedupWindow)
}

// TestLogLineProcessorEventDedup verifies that the logLineProcessor deduplicates
// repeated events with different timestamps but the same name+message within the
// event dedup window. This is the scenario described in the bug: the same
// fabric-manager error repeats every second, each with a new timestamp.
func TestLogLineProcessorEventDedup(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan logLine, 100)
	mw := &mockWatcher{ch: ch}

	llp := newLogLineProcessor(ctx, mw, Match, bucket)
	defer llp.close()

	// Simulate 60 topology mismatch errors arriving every second
	// (mimics the reproduction script from the bug report).
	base := time.Date(2026, time.April, 1, 16, 37, 0, 0, time.UTC)
	for i := range 60 {
		ch <- logLine{
			ts:      base.Add(time.Duration(i) * time.Second),
			content: "[ERROR] [tid 99999] detected number of NVSwitches don't match with any supported system topology, aborting fabric manager",
		}
	}

	// Give the processor goroutine time to drain the channel and insert events.
	require.Eventually(t, func() bool {
		events, err := bucket.Get(ctx, base.Add(-time.Minute))
		if err != nil {
			return false
		}
		// At least one event should be present.
		return len(events) >= 1
	}, 5*time.Second, 50*time.Millisecond, "expected at least one event to be inserted")

	// Verify only 1 event was inserted (all duplicates were deduped).
	events, err := bucket.Get(ctx, base.Add(-time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 1, len(events), "expected exactly 1 event after dedup, got %d", len(events))
	assert.Equal(t, eventNVSwitchTopologyMismatch, events[0].Name)
	assert.Equal(t, messageNVSwitchTopologyMismatch, events[0].Message)
}

// TestLogLineProcessorEventDedupDifferentEvents verifies that distinct event
// types are NOT suppressed by the event deduper.
func TestLogLineProcessorEventDedupDifferentEvents(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)

	bucket, err := store.Bucket(Name)
	require.NoError(t, err)
	defer bucket.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan logLine, 100)
	mw := &mockWatcher{ch: ch}

	llp := newLogLineProcessor(ctx, mw, Match, bucket)
	defer llp.close()

	base := time.Date(2026, time.April, 1, 16, 37, 0, 0, time.UTC)

	// Send two different event types
	ch <- logLine{
		ts:      base,
		content: "[ERROR] [tid 99999] detected number of NVSwitches don't match with any supported system topology, aborting fabric manager",
	}
	ch <- logLine{
		ts:      base.Add(time.Second),
		content: "[ERROR] [tid 841] detected NVSwitch fatal error 20034 on fid 0 on NVSwitch pci bus id 00000000:86:00.0 physical id 3 port 33",
	}

	// Wait for both events to be processed.
	require.Eventually(t, func() bool {
		events, err := bucket.Get(ctx, base.Add(-time.Minute))
		if err != nil {
			return false
		}
		return len(events) >= 2
	}, 5*time.Second, 50*time.Millisecond, "expected 2 distinct events to be inserted")

	events, err := bucket.Get(ctx, base.Add(-time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 2, len(events), "different event types should not be deduped against each other")
}

func TestDefaultWatchCommandsSyntax(t *testing.T) {
	// regression test to ensure we don't accidentally re-introduce incorrect bash -c usage
	assert.Equal(t, 1, len(defaultWatchCommands), "should have 1 command group")
	assert.Equal(t, 1, len(defaultWatchCommands[0]), "should have 1 command string")
	assert.False(t, strings.HasPrefix(defaultWatchCommands[0][0], "bash"), "should not start with bash (process library handles wrapping)")
	assert.Contains(t, defaultWatchCommands[0][0], "&", "should contain background operator")
	assert.Contains(t, defaultWatchCommands[0][0], "wait", "should contain wait")
}
