package diagnose

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestTruncateKeepEnd(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	// Create content: 100 lines, each line is about 10KB
	lineContent := strings.Repeat("This is a test line. ", 500) // ~10KB per line
	var content strings.Builder
	for i := 0; i < 100; i++ {
		content.WriteString(fmt.Sprintf("Line %d: %s\n", i+1, lineContent))
	}

	// Write content to the file
	if _, err := tmpfile.WriteString(content.String()); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}
	tmpfile.Close()

	// Get file info
	fileInfo, err := os.Stat(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to get file info: %v", err)
	}

	// Ensure the file is around 1MB
	if fileInfo.Size() < 1000000 || fileInfo.Size() > 1100000 {
		t.Fatalf("file size is not approximately 1MB: %d bytes", fileInfo.Size())
	}

	// Calculate size for last 50 lines (approximate)
	sizeFor50Lines := int64(50 * (len(lineContent) + len("Line 100: \n")))

	if err = truncateKeepEnd(tmpfile.Name(), sizeFor50Lines); err != nil {
		t.Fatalf("truncateKeepEnd() error = %v", err)
	}

	// Read the file content after truncation
	contentBytes, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}

	lines := strings.Split(string(contentBytes), "\n")
	if len(lines) != 52 {
		t.Errorf("expected 52 lines, got %d", len(lines))
	}

	lastLine := lines[len(lines)-2]
	if !strings.Contains(lastLine, "Line 100") {
		t.Errorf("last line does not contain 'Line 100': %s", lastLine)
	}
}
