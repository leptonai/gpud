package infiniband

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIBStat(t *testing.T) {
	input := `CA 'mlx5_0'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300e3142a
	System image GUID: 0xa088c20300e3142a
	Port 1:
		State: Active
		Physical state: LinkUp
		Rate: 400
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0xa288c2fffee3142a
		Link layer: Ethernet`

	parsed, err := ParseIBStat(input)
	if err != nil {
		t.Fatalf("Failed to parse ibstat output: %v", err)
	}
	t.Logf("parsed:\n%+v", parsed)
}

func TestParseIBStatFiles(t *testing.T) {
	files, err := filepath.Glob("testdata/ibstat.*")
	if err != nil {
		t.Fatalf("Failed to get ibstat files: %v", err)
	}
	for _, file := range files {
		t.Logf("file: %s", file)
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", file, err)
		}
		if _, err := ParseIBStat(string(content)); err != nil {
			t.Fatalf("Failed to parse ibstat file %s: %v", file, err)
		}
	}
}

func TestParseIBStatCountByRates(t *testing.T) {
	tt := []struct {
		fileName              string
		rate                  int
		expectedState         string
		expectedPhysicalState string
		expectedCount         int
	}{
		{
			fileName:              "testdata/ibstat.47.0.a100.all.active.0",
			rate:                  200,
			expectedState:         "Active",
			expectedPhysicalState: "LinkUp",
			expectedCount:         9,
		},
		{
			fileName:              "testdata/ibstat.47.0.a100.all.active.0",
			rate:                  100,
			expectedState:         "Active",
			expectedPhysicalState: "LinkUp",
			expectedCount:         9,
		},
		{
			fileName:              "testdata/ibstat.47.0.h100.all.active.0",
			rate:                  400,
			expectedState:         "Active",
			expectedPhysicalState: "LinkUp",
			expectedCount:         8,
		},
		{
			fileName:              "testdata/ibstat.47.0.h100.all.active.1",
			rate:                  400,
			expectedState:         "Active",
			expectedPhysicalState: "LinkUp",
			expectedCount:         8,
		},
		{
			fileName:              "testdata/ibstat.47.0.h100.some.down.0",
			rate:                  400,
			expectedState:         "Active",
			expectedPhysicalState: "LinkUp",
			expectedCount:         8,
		},
		{
			fileName:              "testdata/ibstat.47.0.h100.some.down.1",
			rate:                  400,
			expectedState:         "Active",
			expectedPhysicalState: "LinkUp",
			expectedCount:         6,
		},
	}
	for _, tc := range tt {
		t.Run(tc.fileName, func(t *testing.T) {
			content, err := os.ReadFile(tc.fileName)
			if err != nil {
				t.Fatalf("Failed to read file %s: %v", tc.fileName, err)
			}
			parsed, err := ParseIBStat(string(content))
			if err != nil {
				t.Fatalf("Failed to parse ibstat file %s: %v", tc.fileName, err)
			}
			count := parsed.CountByRates(tc.rate, tc.expectedState, tc.expectedPhysicalState)
			if count != tc.expectedCount {
				t.Errorf("Expected %d cards, got %d", tc.expectedCount, count)
			}
		})
	}
}
