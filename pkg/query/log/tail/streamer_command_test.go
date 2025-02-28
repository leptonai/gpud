package tail

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestCommandStreamer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tmpf, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	streamer, err := NewFromCommand(ctx, [][]string{{"tail", "-f", tmpf.Name()}})
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
			t.Logf("received %q", line.Text)
			if line.Text != testLine {
				t.Fatalf("expected %q, got %q", testLine, line.Text)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout")
		}
	}

	t.Logf("%+v\n", streamer.Commands())
}

func TestCommandStreamerWithDedup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tmpf, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	streamer, err := NewFromCommand(
		ctx,
		[][]string{{"tail", "-f", tmpf.Name()}},
		WithDedup(true),
	)
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

	// Should only receive one line despite writing multiple
	select {
	case line := <-streamer.Line():
		t.Logf("received %q", line.Text)
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

	t.Logf("%+v\n", streamer.Commands())
}

func readFileToLines(t *testing.T, path string) []string {
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}
