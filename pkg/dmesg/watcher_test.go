package dmesg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/pkg/process"
)

func TestWatch(t *testing.T) {
	w, err := NewWatcherWithCommands([][]string{{"echo 123"}})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	ch := w.Watch()
	for logLine := range ch {
		if logLine.Content != "123" {
			t.Fatalf("expected content '123', got %s", logLine.Content)
		}
	}
}

func TestWatchDmesgLogs(t *testing.T) {
	// sleep 5 seconds to stream the whole file before command exit
	w, err := NewWatcherWithCommands([][]string{
		{"cat testdata/dmesg.decode.iso.log.0"},
		{"cat testdata/dmesg.decode.iso.log.1"},
		{"sleep 7"},
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	for logLine := range w.Watch() {
		if logLine.Facility != "kern" && logLine.Facility != "daemon" && logLine.Facility != "syslog" && logLine.Facility != "user" {
			t.Fatalf("unexpected facility %+v", logLine)
		}
		if logLine.Level != "notice" && logLine.Level != "info" && logLine.Level != "debug" && logLine.Level != "warn" && logLine.Level != "err" {
			t.Fatalf("unexpected level %+v", logLine)
		}
		if logLine.Content == "" {
			t.Fatalf("should see non-empty content %+v", logLine)
		}
	}
}

func TestParseDmesgLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     LogLine
		wantTime bool
	}{
		{
			name:  "normal log line",
			input: "kern  :warn  : 2025-01-21T04:41:44,285060+00:00 Test message",
			want: LogLine{
				Timestamp: time.Date(2025, time.January, 21, 4, 41, 44, 285060000, time.UTC),
				Facility:  "kern",
				Level:     "warn",
				Content:   "Test message",
			},
			wantTime: true,
		},
		{
			name:  "empty line",
			input: "",
			want: LogLine{
				Timestamp: time.Time{},
				Content:   "",
			},
			wantTime: false,
		},
		{
			name:  "no timestamp",
			input: "kern:warn: some message",
			want: LogLine{
				Timestamp: time.Time{},
				Facility:  "",
				Level:     "",
				Content:   "kern:warn: some message",
			},
			wantTime: false,
		},
		{
			name:  "malformed timestamp",
			input: "kern:warn: 2024-13-45T99:99:99 invalid time",
			want: LogLine{
				Timestamp: time.Time{},
				Facility:  "",
				Level:     "",
				Content:   "kern:warn: 2024-13-45T99:99:99 invalid time",
			},
			wantTime: false,
		},
		{
			name:  "no facility or level",
			input: "2025-01-21T04:26:49,785441+00:00 pure message",
			want: LogLine{
				Timestamp: time.Date(2025, time.January, 21, 4, 26, 49, 785441000, time.UTC),
				Content:   "pure message",
			},
			wantTime: true,
		},
		{
			name:  "nvidia xid error",
			input: "kern  :warn  : 2025-01-21T04:26:49,803751+00:00 NVRM: Xid (PCI:0000:38:00): 13, pid='<unknown>', name=<unknown>, Graphics SM Global Exception on (GPC 9, TPC 1, SM 1): Multiple Warp Errors",
			want: LogLine{
				Timestamp: time.Date(2025, time.January, 21, 4, 26, 49, 803751000, time.UTC),
				Facility:  "kern",
				Level:     "warn",
				Content:   "NVRM: Xid (PCI:0000:38:00): 13, pid='<unknown>', name=<unknown>, Graphics SM Global Exception on (GPC 9, TPC 1, SM 1): Multiple Warp Errors",
			},
			wantTime: true,
		},
		{
			name:  "multiple colons in message",
			input: "kern  :info  : 2025-01-21T04:26:49,803751+00:00 systemd[1]: Starting: test:service:name",
			want: LogLine{
				Timestamp: time.Date(2025, time.January, 21, 4, 26, 49, 803751000, time.UTC),
				Facility:  "kern",
				Level:     "info",
				Content:   "systemd[1]: Starting: test:service:name",
			},
			wantTime: true,
		},
		{
			name:  "empty level with facility",
			input: "kern  :  : 2025-01-21T04:26:49,803751+00:00 test message",
			want: LogLine{
				Timestamp: time.Date(2025, time.January, 21, 4, 26, 49, 803751000, time.UTC),
				Facility:  "kern",
				Content:   "test message",
			},
			wantTime: true,
		},
		{
			name:  "invalid timestamp format but valid prefix",
			input: "kern  :warn  : 2025-01-21T04:26:49 test message",
			want: LogLine{
				Timestamp: time.Time{},
				Content:   "kern  :warn  : 2025-01-21T04:26:49 test message",
			},
			wantTime: false,
		},
		{
			name:  "timestamp parse error",
			input: "kern  :warn  : 2025-01-21T25:61:99,803751+00:00 test message",
			want: LogLine{
				Facility: "",
				Level:    "",
				Content:  "kern  :warn  : 2025-01-21T25:61:99,803751+00:00 test message",
			},
			wantTime: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDmesgLine(tt.input)

			if tt.wantTime && !got.Timestamp.Equal(tt.want.Timestamp) {
				t.Errorf("Timestamp = %v, want %v", got.Timestamp, tt.want.Timestamp)
			}
			if got.Facility != tt.want.Facility {
				t.Errorf("Facility = %v, want %v", got.Facility, tt.want.Facility)
			}
			if got.Level != tt.want.Level {
				t.Errorf("Level = %v, want %v", got.Level, tt.want.Level)
			}
			if got.Content != tt.want.Content {
				t.Errorf("Content = %v, want %v", got.Content, tt.want.Content)
			}
			if tt.wantTime && got.Timestamp.IsZero() {
				t.Error("Expected non-zero timestamp")
			}
		})
	}
}

func TestParseDmesgLineWhitespaceDedup(t *testing.T) {
	// Create log lines with same content but different trailing whitespaces
	logLines := []string{
		"kern  :err   : 2025-02-10T16:28:06,502716+00:00 nvidia-peermem error message",
		"kern  :err   : 2025-02-10T16:28:06,514050+00:00 nvidia-peermem error message  ",   // two spaces at end
		"kern  :err   : 2025-02-10T16:28:06,525389+00:00 nvidia-peermem error message\t",   // tab at end
		"kern  :err   : 2025-02-10T16:28:06,535389+00:00 nvidia-peermem error message \t ", // mixed whitespace at end
	}

	var parsedLines []LogLine
	for _, line := range logLines {
		parsedLines = append(parsedLines, ParseDmesgLine(line))
	}

	// Verify all lines have the same content after trimming whitespace
	expectedContent := "nvidia-peermem error message"
	for i, line := range parsedLines {
		assert.Equal(t, expectedContent, strings.TrimSpace(line.Content),
			"line %d should have same content after trimming whitespace", i)
	}

	// Verify all lines have the same cache key
	firstKey := parsedLines[0].cacheKey()
	for i, line := range parsedLines[1:] {
		assert.Equal(t, firstKey, line.cacheKey(),
			"line %d should have same cache key as first line", i+1)
	}

	// Verify facility and level are parsed correctly for all lines
	for i, line := range parsedLines {
		assert.Equal(t, "kern", line.Facility, "line %d should have correct facility", i)
		assert.Equal(t, "err", line.Level, "line %d should have correct level", i)
	}

	// Verify timestamps are parsed correctly and all from the same second
	expectedSecond := time.Date(2025, 2, 10, 16, 28, 6, 0, time.UTC).Unix()
	for i, line := range parsedLines {
		assert.Equal(t, expectedSecond, line.Timestamp.Unix(),
			"line %d should have correct second timestamp", i)
	}
}

func TestWatcherClose(t *testing.T) {
	w, err := NewWatcherWithCommands([][]string{
		{"sleep 10"},
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	ch := w.Watch()
	// Give some time for goroutines to start
	time.Sleep(100 * time.Millisecond)
	w.Close()

	// Wait for channel to close
	for range ch {
		// Drain the channel
	}
}

func TestWatcherError(t *testing.T) {
	tests := []struct {
		name    string
		cmds    [][]string
		wantErr bool
	}{
		{
			name:    "no commands",
			cmds:    [][]string{},
			wantErr: true,
		},
		{
			name:    "nil commands",
			cmds:    nil,
			wantErr: true,
		},
		{
			name:    "non-existent command",
			cmds:    [][]string{{"nonexistentcommand"}},
			wantErr: true,
		},
		{
			name:    "valid command",
			cmds:    [][]string{{"echo test"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewWatcherWithCommands(tt.cmds)
			if (err != nil) != tt.wantErr {
				t.Errorf("newWatcher() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := watch(ctx,
		[][]string{
			{"sleep 10"},
		},
		DefaultCacheExpiration,
		DefaultCachePurgeInterval,
	)
	if err != nil {
		t.Fatalf("failed to create watch: %v", err)
	}

	// Give some time for goroutines to start
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for channel to close
	for range ch {
		// Drain the channel
	}
}

func TestContextCancellationWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ch, err := watch(
		ctx,
		[][]string{
			{"sleep 10"},
		},
		DefaultCacheExpiration,
		DefaultCachePurgeInterval,
	)
	if err != nil {
		t.Fatalf("failed to create watch: %v", err)
	}

	// Wait for channel to close due to timeout
	start := time.Now()
	for range ch {
		// Drain the channel
	}
	duration := time.Since(start)

	// Should complete around the timeout duration
	if duration > 200*time.Millisecond {
		t.Errorf("watch took too long to cancel: %v", duration)
	}
}

func TestContextCancellationWithMultipleCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := watch(
		ctx,
		[][]string{
			{"echo 'first'"},
			{"sleep 1"},
			{"echo 'second'"},
			{"sleep 10"},
		},
		DefaultCacheExpiration,
		DefaultCachePurgeInterval,
	)
	if err != nil {
		t.Fatalf("failed to create watch: %v", err)
	}

	var lines []string
	timer := time.NewTimer(200 * time.Millisecond)
	go func() {
		<-timer.C
		cancel()
	}()

	for line := range ch {
		lines = append(lines, line.Content)
	}

	// Should see 'first' but might not see 'second' due to cancellation
	found := false
	for _, line := range lines {
		if line == "first" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to see 'first' in output before cancellation")
	}
}

func TestFindISOTimestampIndex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "invalid month/hour",
			input: "2023-13-01T25:00:00 invalid date",
			want:  -1,
		},
		{
			name:  "valid timestamp",
			input: "kern  :info  : 2025-01-17T15:36:17,173085+00:00",
			want:  15,
		},
		{
			name:  "invalid timestamp with message",
			input: "  2024-02-29T15:30:00 some message",
			want:  -1,
		},
		{
			name:  "no timestamp",
			input: "no timestamp here",
			want:  -1,
		},
		{
			name:  "shorter timestamp",
			input: "kern  :info  : 2025-01-17T15:36:11",
			want:  -1,
		},
		{
			name:  "valid timestamp but shorter",
			input: "prefix 2024-01-21T04:41:44 suffix",
			want:  -1,
		},
		{
			name:  "valid timestamp",
			input: "prefix 2025-01-17T15:36:17,173085+00:00 suffix",
			want:  7,
		},
		{
			name:  "no timestamp",
			input: "no timestamp here",
			want:  -1,
		},
		{
			name:  "malformed timestamp",
			input: "2024-13-45T99:99:99",
			want:  -1,
		},
		{
			name:  "empty string",
			input: "",
			want:  -1,
		},
		{
			name:  "timestamp at start but shorter",
			input: "2024-01-21T04:41:44 message",
			want:  -1,
		},
		{
			name:  "multiple timestamps but shorter",
			input: "2024-01-21T04:41:44 and 2024-01-21T04:41:45",
			want:  -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findISOTimestampIndex(tt.input)
			if got != tt.want {
				t.Errorf("findISOTimestampIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindTimestampIndexFromFiles(t *testing.T) {
	t.Parallel()

	dir, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to read testdata dir: %v", err)
	}

	for _, entry := range dir {
		if entry.IsDir() {
			continue
		}

		b, err := os.ReadFile(filepath.Join("testdata", entry.Name()))
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		lines := strings.Split(string(b), "\n")
		for _, line := range lines {
			if len(line) == 0 {
				continue
			}

			idx := findISOTimestampIndex(line)
			if idx == -1 {
				t.Logf("file %s: %d %q", entry.Name(), idx, line)
			}

			// should never happen
			if idx != -1 && len(line) < len(isoFormat) {
				t.Errorf("file %s: %d %q", entry.Name(), len(line), line)
			}
		}
	}

}

func TestWatchMultipleCommands(t *testing.T) {
	// wait for some time to be read
	// slow CI
	w, err := NewWatcherWithCommands(
		[][]string{
			{"echo 'first command'"},
			{"echo 'second command'"},
			{"sleep 5"},
		},
	)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	ch := w.Watch()

	var lines []string
	for line := range ch {
		lines = append(lines, line.Content)
	}

	output := strings.Join(lines, "\n")
	if !strings.Contains(output, "first command") {
		t.Errorf("expected 'first command' in output, got %s", output)
	}
	if !strings.Contains(output, "second command") {
		t.Errorf("expected 'second command' in output, got %s", output)
	}
}

func TestWatchWithError(t *testing.T) {
	tests := []struct {
		name        string
		commands    [][]string
		wantError   bool
		wantContent string
	}{
		{
			name:        "invalid command",
			commands:    [][]string{{"invalidcommand"}},
			wantError:   true,
			wantContent: "not found",
		},
		{
			name:        "command with no output",
			commands:    [][]string{{"true"}},
			wantError:   false,
			wantContent: "",
		},
		{
			name:        "command that fails",
			commands:    [][]string{{"false"}},
			wantError:   false,
			wantContent: "",
		},
		{
			name: "multiple failing commands",
			commands: [][]string{
				{"false"},
				{"invalidcommand"},
				{"cat", "nonexistentfile"},
			},
			wantError:   true,
			wantContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := NewWatcherWithCommands(tt.commands)
			if err != nil {
				if !tt.wantError {
					t.Errorf("NewWatcherWithCommands() error = %v, wantError %v", err, tt.wantError)
				}
				return
			}
			defer w.Close()

			ch := w.Watch()
			var errorSeen bool
			var lastError string

			for line := range ch {
				if line.Error != "" {
					errorSeen = true
					lastError = line.Error
				}
			}

			if tt.wantError && !errorSeen {
				t.Error("expected to see an error line but got none")
			}
			if tt.wantContent != "" && !strings.Contains(lastError, tt.wantContent) {
				t.Errorf("expected error to contain %q, got %q", tt.wantContent, lastError)
			}
		})
	}
}

func TestWatcherCloseMultipleTimes(t *testing.T) {
	w, err := NewWatcherWithCommands([][]string{{"sleep", "10"}})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	ch := w.Watch()
	// Give some time for goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Call Close multiple times to ensure it's safe
	for i := 0; i < 3; i++ {
		w.Close()
	}

	// Wait for channel to close
	for range ch {
		// Drain the channel
	}

	// Verify channel is closed by trying to read again
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed")
	}
}

func TestWatchWithLongOutput(t *testing.T) {
	// Generate a command that produces a lot of output
	w, err := NewWatcherWithCommands([][]string{
		{"bash", "-c", "for i in {1..1000}; do echo $i; done"},
	})
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	ch := w.Watch()
	count := 0
	for range ch {
		count++
	}

	if count == 0 {
		t.Error("expected to receive some output")
	}
}

func TestReadContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan LogLine, 1000)

	// Create a command that will run for a while
	p, err := process.New(process.WithCommand("sleep", "10"))
	if err != nil {
		t.Fatalf("failed to create process: %v", err)
	}
	if err := p.Start(ctx); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	// Start reading in a goroutine
	go read(ctx, p, DefaultCacheExpiration, DefaultCachePurgeInterval, ch)

	// Give it a moment to start
	time.Sleep(time.Second)

	// Cancel the context
	cancel()

	// Wait for the channel to close
	timer := time.NewTimer(1 * time.Second)
	select {
	case _, ok := <-ch:
		if !ok {
			// Channel closed as expected
			return
		}

		// just log for slow CI
		t.Log("channel should have been closed after context cancellation")

	case <-timer.C:
		t.Error("timeout waiting for channel to close after context cancellation")
	}
}
