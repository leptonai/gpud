package dmesg

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestWatch(t *testing.T) {
	w, err := newWatcher("echo 123")
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

func TestDmesgLogs(t *testing.T) {
	// sleep 3 seconds to stream the whole file before command exit
	w, err := newWatcher("cat testdata/dmesg.decode.iso.log.0 && sleep 3")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	for logLine := range w.Watch() {
		if logLine.Facility != "kern" {
			t.Fatalf("should see kern facility %+v", logLine)
		}
		if logLine.Level != "warn" {
			t.Fatalf("should see warn level %+v", logLine)
		}
		if logLine.Content == "" {
			t.Fatalf("should see content %+v", logLine)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDmesgLine(tt.input)

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

func TestWatcherClose(t *testing.T) {
	w, err := newWatcher("sleep 10")
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
		cmds    []string
		wantErr bool
	}{
		{
			name:    "no commands",
			cmds:    []string{},
			wantErr: true,
		},
		{
			name:    "nil commands",
			cmds:    nil,
			wantErr: true,
		},
		{
			name:    "non-existent command",
			cmds:    []string{"nonexistentcommand"},
			wantErr: true,
		},
		{
			name:    "valid command",
			cmds:    []string{"echo test"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newWatcher(tt.cmds...)
			if (err != nil) != tt.wantErr {
				t.Errorf("newWatcher() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := watch(ctx, "sleep 10")
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

func TestFindISOTimestampIndex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "valid timestamp",
			input: "prefix 2024-01-21T04:41:44 suffix",
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
			name:  "timestamp at start",
			input: "2024-01-21T04:41:44 message",
			want:  0,
		},
		{
			name:  "multiple timestamps",
			input: "2024-01-21T04:41:44 and 2024-01-21T04:41:45",
			want:  0,
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

func TestWatchMultipleCommands(t *testing.T) {
	w, err := newWatcher(
		"echo 'first command'",
		"echo 'second command'",
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
	w, err := newWatcher("cat nonexistentfile")
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer w.Close()

	ch := w.Watch()

	var errorSeen bool
	for line := range ch {
		if strings.Contains(line.Content, "No such file or directory") {
			errorSeen = true
		}
	}

	if !errorSeen {
		t.Error("expected to see an error line")
	}
}

func TestFindTimestampIndex(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		input    string
		expected int
	}{
		{
			input:    "2025-01-21T04:41:44 some message",
			expected: 0,
		},
		{
			input:    "prefix 2023-12-31T23:59:59 some message",
			expected: 7,
		},
		{
			input:    "  2024-02-29T15:30:00 some message",
			expected: 2,
		},
		{
			input:    "no timestamp here",
			expected: -1,
		},
		{
			input:    "",
			expected: -1,
		},
		{
			input:    "2023-13-01T25:00:00 invalid date", // invalid month/hour
			expected: -1,
		},
		{
			input:    "partial 2023-01-01 timestamp", // missing time part
			expected: -1,
		},
	}

	for _, tc := range testCases {
		idx := findISOTimestampIndex(tc.input)
		if idx != tc.expected {
			t.Errorf("Expected index %d for input %q, got %d", tc.expected, tc.input, idx)
		}
	}
}
