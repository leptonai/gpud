package tail

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
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
