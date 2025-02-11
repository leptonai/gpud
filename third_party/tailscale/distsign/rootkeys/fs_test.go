package rootkeys

import (
	"testing"
)

func TestRootsFS(t *testing.T) {
	// Test that we can list the embedded files
	entries, err := RootsFS.ReadDir("keys")
	if err != nil {
		t.Fatalf("failed to read keys directory: %v", err)
	}

	// Verify we have exactly 2 files
	if len(entries) != 2 {
		t.Errorf("expected 2 files, got %d", len(entries))
	}

	// Expected files
	expectedFiles := map[string]bool{
		"gpud-root-1.pem": false,
		"gpud-root-2.pem": false,
	}

	// Verify each file exists and can be read
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := expectedFiles[name]; !ok {
			t.Errorf("unexpected file: %s", name)
			continue
		}
		expectedFiles[name] = true

		// Try to read the file
		content, err := RootsFS.ReadFile("keys/" + name)
		if err != nil {
			t.Errorf("failed to read file %s: %v", name, err)
			continue
		}

		// Verify the file is not empty
		if len(content) == 0 {
			t.Errorf("file %s is empty", name)
		}
	}

	// Verify all expected files were found
	for name, found := range expectedFiles {
		if !found {
			t.Errorf("expected file %s was not found", name)
		}
	}
}

func TestRootsFSErrors(t *testing.T) {
	// Test reading non-existent directory
	_, err := RootsFS.ReadDir("nonexistent")
	if err == nil {
		t.Error("expected error when reading non-existent directory")
	}

	// Test reading non-existent file
	_, err = RootsFS.ReadFile("keys/nonexistent.pem")
	if err == nil {
		t.Error("expected error when reading non-existent file")
	}

	// Test reading a directory as a file
	_, err = RootsFS.ReadFile("keys")
	if err == nil {
		t.Error("expected error when reading directory as file")
	}
}
