package tail

import (
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

	streamer, err := NewFromFile(tmpf.Name(), nil)
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
