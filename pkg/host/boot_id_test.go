package host

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetBootID(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "boot_id_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	existingFilePath := filepath.Join(tempDir, "boot_id")
	noPermFilePath := filepath.Join(tempDir, "no_perm_boot_id")
	whitespaceFilePath := filepath.Join(tempDir, "whitespace_boot_id")

	// Write content to test files
	if err := os.WriteFile(existingFilePath, []byte("boot-id-123"), 0644); err != nil {
		t.Fatalf("Failed to write to test file: %v", err)
	}
	if err := os.WriteFile(noPermFilePath, []byte("no-perm-boot-id"), 0000); err != nil {
		t.Fatalf("Failed to write to test file: %v", err)
	}
	if err := os.WriteFile(whitespaceFilePath, []byte("  boot-id-with-whitespace  \n"), 0644); err != nil {
		t.Fatalf("Failed to write to test file: %v", err)
	}

	nonExistentPath := filepath.Join(tempDir, "does_not_exist")

	tests := []struct {
		name     string
		file     string
		expected string
		wantErr  bool
	}{
		{
			name:     "File exists",
			file:     existingFilePath,
			expected: "boot-id-123",
			wantErr:  false,
		},
		{
			name:     "File doesn't exist",
			file:     nonExistentPath,
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Permission denied",
			file:     noPermFilePath,
			expected: "",
			wantErr:  true,
		},
		{
			name:     "Whitespace trimming",
			file:     whitespaceFilePath,
			expected: "boot-id-with-whitespace",
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readBootID(tc.file)

			if (err != nil) != tc.wantErr {
				t.Errorf("getBootID() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if got != tc.expected {
				t.Errorf("getBootID() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestGetBootIDWithTestData(t *testing.T) {
	// Create a test file in the testdata directory
	testFile := "testdata/boot_id_sample"

	if err := os.WriteFile(testFile, []byte("sample-boot-id-12345"), 0644); err != nil {
		t.Fatalf("Failed to write to test file: %v", err)
	}
	defer os.Remove(testFile)

	id, err := readBootID(testFile)
	if err != nil {
		t.Fatalf("Failed to get boot ID from test file: %v", err)
	}

	expected := "sample-boot-id-12345"
	if id != expected {
		t.Errorf("getBootID() = %q, want %q", id, expected)
	}
}
