package query

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCountDevEntry(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "Valid device with /dev prefix",
			input:    "/dev/nvidia0",
			expected: true,
		},
		{
			name:     "Valid device with /dev prefix and different number",
			input:    "/dev/nvidia1",
			expected: true,
		},
		{
			name:     "Valid device without /dev prefix",
			input:    "/nvidia2",
			expected: true,
		},
		{
			name:     "Valid device without /dev prefix and different number",
			input:    "/nvidia3",
			expected: true,
		},
		{
			name:     "Invalid device without number",
			input:    "nvidia",
			expected: false,
		},
		{
			name:     "Invalid device with non-numeric suffix",
			input:    "nvidiax",
			expected: false,
		},
		{
			name:     "Invalid device with prefix",
			input:    "my_nvidia0",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := countDevEntry(tc.input)
			if result != tc.expected {
				t.Errorf("countDevEntry(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestCountAllDevicesFromDir(t *testing.T) {
	testDir := t.TempDir()
	defer t.Cleanup(func() {
		_ = os.RemoveAll(testDir)
	})

	devCnt := 8
	for i := 0; i < devCnt; i++ {
		fileName := filepath.Join(testDir, fmt.Sprintf("nvidia%d", i))
		_, err := os.Create(fileName)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", fileName, err)
		}
	}

	count, err := countAllDevicesFromDir(testDir)
	if err != nil {
		t.Fatalf("countAllDevicesFromDir returned an error: %v", err)
	}

	if count != devCnt {
		t.Errorf("expected %d devices, but got %d", devCnt, count)
	}
}

func TestCountAllDevicesFromDevDir(t *testing.T) {
	devCnt, err := CountAllDevicesFromDevDir()
	if err != nil {
		t.Fatalf("CountAllDevicesFromDevDir returned an error: %v", err)
	}
	t.Logf("CountAllDevicesFromDevDir: %d", devCnt)
}
