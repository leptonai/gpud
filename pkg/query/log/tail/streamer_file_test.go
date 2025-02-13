package tail

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

func TestFileStreamer(t *testing.T) {
	tmpf, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	streamer, err := NewFromFile(ctx, tmpf.Name(), nil)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	for i := 0; i < 10; i++ {
		testLine := fmt.Sprintf("%d%d", i, time.Now().Nanosecond())
		if _, err := tmpf.WriteString(testLine + "\n"); err != nil {
			t.Fatal(err)
		}

		select {
		case line := <-streamer.Line():
			t.Logf("received %q (%v, %+v)", line.Text, line.Time, line.SeekInfo)
			if line.Text != testLine {
				t.Fatalf("expected %q, got %q", testLine, line.Text)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestFileStreamerWithDedup(t *testing.T) {
	tmpf, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	streamer, err := NewFromFile(ctx, tmpf.Name(), nil, WithDedup(true))
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	// Write same line multiple times
	testLine := "duplicate line"
	for i := 0; i < 10; i++ {
		if _, err := tmpf.WriteString(testLine + "\n"); err != nil {
			t.Fatal(err)
		}
	}

	// Should only receive one line despite writing three
	select {
	case line := <-streamer.Line():
		t.Logf("received %q (%v, %+v)", line.Text, line.Time, line.SeekInfo)
		if line.Text != testLine {
			t.Fatalf("expected %q, got %q", testLine, line.Text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for first line")
	}

	// Verify no more lines are received (as they should be deduped)
	select {
	case line := <-streamer.Line():
		t.Fatalf("unexpected line received: %q", line.Text)
	case <-time.After(2 * time.Second):
		// This is the expected path - no additional lines should be received
	}
}

func TestFileStreamerWithExtractTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamer, err := NewFromFile(ctx, "testdata/fabric-manager.0.log", nil, WithExtractTime(extractTimeFromLogLine), WithSkipEmptyLine(true))
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	for i := 0; i < 30; i++ {
		select {
		case line := <-streamer.Line():
			t.Logf("received %q (%v, %+v)", line.Text, line.Time, line.SeekInfo)

			// "[Dec 18 2024"
			if line.Time.IsZero() {
				t.Fatalf("expected non-zero time, got %v", line.Time)
			}
			if line.Time.Year() != 2024 {
				t.Fatalf("expected 2024, got %v", line.Time.Year())
			}
			if line.Time.Month() != time.December {
				t.Fatalf("expected December, got %v", line.Time.Month())
			}
			if line.Time.Day() < 18 || line.Time.Day() > 20 {
				t.Fatalf("expected day between 18 and 20, got %v", line.Time.Day())
			}

		case <-time.After(3 * time.Second):
			t.Fatal("timeout")
		}
	}
}

var regexForFabricmanagerLog = regexp.MustCompile(`^\[([^\]]+)\]`)

const fabricmanagerLogTimeFormat = "Jan 02 2006 15:04:05"

var fabricmanagerLogTimeFormatN = len(fabricmanagerLogTimeFormat) + 2 // [ ]

// does not return error for now
// example log line: "[May 02 2024 18:41:23] [INFO] [tid 404868] Abort CUDA jobs when FM exits = 1"
// TODO: once stable return error
func extractTimeFromLogLine(line []byte) (time.Time, []byte, error) {
	matches := regexForFabricmanagerLog.FindStringSubmatch(string(line))
	if len(matches) == 0 {
		log.Logger.Debugw("no timestamp matches found", "line", string(line))
		return time.Time{}, nil, nil
	}

	s := matches[1]

	parsedTime, err := time.Parse("Jan 02 2006 15:04:05", s)
	if err != nil {
		log.Logger.Debugw("failed to parse timestamp", "line", string(line), "error", err)
		return time.Time{}, nil, nil
	}

	if len(line) <= fabricmanagerLogTimeFormatN {
		return parsedTime, nil, nil
	}

	extractedLine := bytes.TrimSpace(line[fabricmanagerLogTimeFormatN:])
	return parsedTime, extractedLine, nil
}
