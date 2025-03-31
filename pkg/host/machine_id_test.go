package host

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

func TestScanUUIDFromDmidecode(t *testing.T) {
	f, err := os.Open("testdata/dmidecode")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	uuid := ""
	for scanner.Scan() {
		uuid = extractUUID(scanner.Text())
		if uuid != "" {
			break
		}
	}
	if uuid != "4c4c4544-0053-5210-8038-c8c04f583034" {
		t.Errorf("expected UUID to be 4c4c4544-0053-5210-8038-c8c04f583034, got %s", uuid)
	}
}

func TestGetOSMachineID(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "machine_id_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	firstFilePath := filepath.Join(tempDir, "first_id")
	secondFilePath := filepath.Join(tempDir, "second_id")
	noPermFilePath := filepath.Join(tempDir, "no_perm_id")

	// Write content to test files
	if err := os.WriteFile(firstFilePath, []byte("machine-id-123\n"), 0644); err != nil {
		t.Fatalf("Failed to write to test file: %v", err)
	}
	if err := os.WriteFile(secondFilePath, []byte("  machine-id-456  "), 0644); err != nil {
		t.Fatalf("Failed to write to test file: %v", err)
	}
	if err := os.WriteFile(noPermFilePath, []byte("no-perm-id"), 0000); err != nil {
		t.Fatalf("Failed to write to test file: %v", err)
	}

	nonExistentPath := filepath.Join(tempDir, "does_not_exist")

	tests := []struct {
		name     string
		files    []string
		expected string
		wantErr  bool
	}{
		{
			name:     "First file exists",
			files:    []string{firstFilePath, secondFilePath},
			expected: "machine-id-123",
			wantErr:  false,
		},
		{
			name:     "Skip non-existent file",
			files:    []string{nonExistentPath, secondFilePath},
			expected: "machine-id-456",
			wantErr:  false,
		},
		{
			name:     "No files exist",
			files:    []string{nonExistentPath, nonExistentPath + "2"},
			expected: "",
			wantErr:  false,
		},
		{
			name:     "Permission denied",
			files:    []string{noPermFilePath},
			expected: "",
			wantErr:  true,
		},
		{
			name:     "Empty file list",
			files:    []string{},
			expected: "",
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readOSMachineID(tc.files)

			if (err != nil) != tc.wantErr {
				t.Errorf("getOSMachineID() error = %v, wantErr %v", err, tc.wantErr)
				return
			}

			if got != tc.expected {
				t.Errorf("getOSMachineID() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestGetOSMachineIDWithTestData(t *testing.T) {
	// Test using the sample file in testdata directory
	files := []string{"testdata/machine_id_sample"}

	id, err := readOSMachineID(files)
	if err != nil {
		t.Fatalf("Failed to get machine ID from test file: %v", err)
	}

	expected := "sample-machine-id-12345"
	if id != expected {
		t.Errorf("getOSMachineID() = %q, want %q", id, expected)
	}
}
