package infiniband

import (
	"errors"
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
		expectedPhysicalState string
		expectedState         string
		expectedAtLeastRate   int
		expectedCount         int
	}{
		{
			fileName:              "testdata/ibstat.47.0.a100.all.active.0",
			expectedPhysicalState: "LinkUp",
			expectedState:         "Active",
			expectedAtLeastRate:   200,
			expectedCount:         9,
		},
		{
			fileName:              "testdata/ibstat.47.0.a100.all.active.0",
			expectedPhysicalState: "LinkUp",
			expectedState:         "Active",
			expectedAtLeastRate:   100,
			expectedCount:         9,
		},
		{
			fileName:              "testdata/ibstat.47.0.h100.all.active.0",
			expectedPhysicalState: "LinkUp",
			expectedState:         "Active",
			expectedAtLeastRate:   400,
			expectedCount:         8,
		},
		{
			fileName:              "testdata/ibstat.47.0.h100.all.active.1",
			expectedPhysicalState: "LinkUp",
			expectedState:         "Active",
			expectedAtLeastRate:   400,
			expectedCount:         8,
		},
		{
			fileName:              "testdata/ibstat.47.0.h100.some.down.0",
			expectedPhysicalState: "LinkUp",
			expectedState:         "Active",
			expectedAtLeastRate:   400,
			expectedCount:         8,
		},
		{
			fileName:              "testdata/ibstat.47.0.h100.some.down.1",
			expectedAtLeastRate:   400,
			expectedPhysicalState: "LinkUp",
			expectedState:         "Active",
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
			count := parsed.Count(
				tc.expectedPhysicalState,
				tc.expectedState,
				tc.expectedAtLeastRate,
			)
			if count != tc.expectedCount {
				t.Errorf("Expected %d cards, got %d", tc.expectedCount, count)
			}
		})
	}
}

func TestValidateIbstatOutputErrIbstatOutputBrokenStateDown(t *testing.T) {
	t.Parallel()

	outputWithErr := `

CA 'mlx5_11'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300bb3514
	System image GUID: 0xa088c20300bb3514
	Port 1:
		State: Down
		Physical state: Disabled
		Rate: 40
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0x0000000000000000
		Link layer: Ethernet
	`
	err := ValidateIbstatOutput(outputWithErr)
	if err != ErrIbstatOutputBrokenStateDown {
		t.Errorf("ibstat output did not pass validation")
	}
}

func TestValidateIbstatOutputErrIbstatOutputBrokenPhysicalDisabled(t *testing.T) {
	t.Parallel()

	outputWithErr := `

CA 'mlx5_11'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300bb3514
	System image GUID: 0xa088c20300bb3514
	Port 1:
		State: Active
		Physical state: Disabled
		Rate: 40
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0x0000000000000000
		Link layer: Ethernet
	`
	err := ValidateIbstatOutput(outputWithErr)
	if err != ErrIbstatOutputBrokenPhysicalDisabled {
		t.Errorf("ibstat output did not pass validation")
	}
}

func TestValidateIbstatOutputHealthyCase(t *testing.T) {
	t.Parallel()

	outputWithNoErr := `

CA 'mlx5_1'
	CA type: MT4125
	Number of ports: 1
	Firmware version: 22.39.1002
	Hardware version: 0
	Node GUID: 0xb83fd203002a1a1c
	System image GUID: 0xb83fd203002a1a1c
	Port 1:
		State: Active
		Physical state: LinkUp
		Rate: 100
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0x0000000000000000
		Link layer: Ethernet

CA 'mlx5_10'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300bb98b4
	System image GUID: 0xa088c20300bb98b4
	Port 1:
		State: Active
		Physical state: LinkUp
		Rate: 400
		Base lid: 0
		LMC: 0
		SM lid: 0
		Capability mask: 0x00010000
		Port GUID: 0xa288c2fffebb98b4
		Link layer: Ethernet
	`
	err := ValidateIbstatOutput(outputWithNoErr)
	if err != nil {
		t.Error("healthy ibstat output did not pass the validation")
	}
}

func TestValidateIBPorts(t *testing.T) {
	tests := []struct {
		name         string
		cards        IBStatCards
		atLeastPorts int
		atLeastRate  int
		wantErr      error
	}{
		{
			name: "all ports active and matching rate",
			cards: IBStatCards{
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
			},
			atLeastPorts: 4,
			atLeastRate:  200,
			wantErr:      nil,
		},
		{
			name: "some ports down",
			cards: IBStatCards{
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: IBStatPort{State: "Down", PhysicalState: "LinkDown", Rate: 200}},
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: IBStatPort{State: "Down", PhysicalState: "LinkDown", Rate: 200}},
			},
			atLeastPorts: 4,
			atLeastRate:  200,
			wantErr:      errors.New("not enough LinkUp ports, some ports must be down; only 2 LinkUp out of 4, expected at least 4 ports and 200 Gb/sec rate"),
		},
		{
			name: "wrong rate",
			cards: IBStatCards{
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100}},
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100}},
			},
			atLeastPorts: 2,
			atLeastRate:  200,
			wantErr:      errors.New("not enough LinkUp ports, less than expected ports with rates, some ports must be missing; only 0 satisfies the expected rate out of 2, expected at least 2 ports and 200 Gb/sec rate"),
		},
		{
			name: "mixed states with empty expected state matches all",
			cards: IBStatCards{
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200}},
				{Port1: IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200}},
				{Port1: IBStatPort{State: "Init", PhysicalState: "LinkUp", Rate: 200}},
			},
			atLeastPorts: 3,
			atLeastRate:  200,
			wantErr:      errors.New("not enough LinkUp ports, some ports must be down; only 2 LinkUp out of 3, expected at least 3 ports and 200 Gb/sec rate"),
		},
		{
			name: "mixed states with wrong rate",
			cards: IBStatCards{
				{Port1: IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100}},
				{Port1: IBStatPort{State: "Down", PhysicalState: "LinkUp", Rate: 100}},
				{Port1: IBStatPort{State: "Init", PhysicalState: "LinkUp", Rate: 100}},
			},
			atLeastPorts: 3,
			atLeastRate:  200,
			wantErr:      errors.New("not enough LinkUp ports, less than expected ports with rates, some ports must be missing; only 0 satisfies the expected rate out of 3, expected at least 3 ports and 200 Gb/sec rate"),
		},
		{
			name:         "empty cards",
			cards:        IBStatCards{},
			atLeastPorts: 2,
			atLeastRate:  200,
			wantErr:      errors.New("not enough LinkUp ports, less than expected ports with rates, some ports must be missing; only 0 satisfies the expected rate out of 0, expected at least 2 ports and 200 Gb/sec rate"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := tt.cards.CheckPortsAndRate(tt.atLeastPorts, tt.atLeastRate)

			if tt.wantErr == nil {
				if gotErr != nil {
					t.Errorf("validateIBPorts() expected no error, got %v", gotErr)
				}
			} else if gotErr == nil || gotErr.Error() != tt.wantErr.Error() {
				t.Errorf("validateIBPorts() expected error:\n%v\n\nwant\n%v", gotErr, tt.wantErr)
			}
		})
	}
}
