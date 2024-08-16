package tail

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
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

func TestCommandStreamerDmesg(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tmpf, err := os.CreateTemp("", "test*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpf.Name())

	re := regexp.MustCompile(`^\[([^\]]+)\]`)

	streamer, err := NewFromCommand(
		ctx,
		[][]string{{"tail", "-f", tmpf.Name()}},
		WithParseTime(func(b []byte) (time.Time, error) {
			matches := re.FindStringSubmatch(string(b))
			if len(matches) == 0 {
				t.Logf("no timestamp matches found for %s", string(b))
				return time.Time{}, nil
			}

			s := matches[1]
			timestamp, err := time.Parse("Mon Jan 2 15:04:05 2006", s)
			if err != nil {
				t.Logf("failed to parse timestamp %s", s)
				return time.Time{}, nil
			}

			return timestamp, nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	for _, line := range readFileToLines(t, "./testdata/dmesg.0.log") {
		if _, err := tmpf.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}

		select {
		case line := <-streamer.Line():
			t.Logf("received %v", line.Time)

			if line.Time.IsZero() {
				t.Fatalf("expected non-zero time, got %v", line.Time)
			}

		case <-time.After(3 * time.Second):
			t.Fatal("timeout")
		}
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
