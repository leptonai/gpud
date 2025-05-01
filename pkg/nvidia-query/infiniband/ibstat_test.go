package infiniband

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetIbstatOutputError(t *testing.T) {
	tests := []struct {
		name           string
		ibstatCommand  []string
		expectedError  error
		expectedOutput string
		wantParsed     bool
		isDynamicError bool // true for errors with dynamic messages (e.g., command execution errors)
	}{
		{
			name:           "no command provided",
			ibstatCommand:  nil,
			expectedError:  ErrNoIbstatCommand,
			expectedOutput: "",
			wantParsed:     false,
			isDynamicError: false,
		},
		{
			name:           "empty command provided",
			ibstatCommand:  []string{" "},
			expectedError:  ErrNoIbstatCommand,
			expectedOutput: "",
			wantParsed:     false,
			isDynamicError: false,
		},
		{
			name:           "command not found in PATH 1",
			ibstatCommand:  []string{"nonexistentcommand"},
			expectedError:  ErrNoIbstatCommand,
			expectedOutput: "",
			wantParsed:     false,
			isDynamicError: false,
		},
		{
			name:           "command not found in PATH 2",
			ibstatCommand:  []string{"nonexistentcommand 123123123123123"},
			expectedError:  ErrNoIbstatCommand,
			expectedOutput: "",
			wantParsed:     false,
			isDynamicError: false,
		},
		{
			name:           "valid output",
			ibstatCommand:  []string{"cat", "testdata/ibstat.47.0.a100.all.active.0"},
			expectedError:  nil,
			expectedOutput: "",
			wantParsed:     true,
			isDynamicError: false,
		},
		{
			name:           "empty output",
			ibstatCommand:  []string{"echo", ""},
			expectedError:  ErrIbstatOutputEmpty,
			expectedOutput: "",
			wantParsed:     false,
			isDynamicError: false,
		},
		{
			name:           "parsing error",
			ibstatCommand:  []string{"echo", "invalid ibstat output"},
			expectedError:  ErrIbstatOutputNoCardFound,
			expectedOutput: "invalid ibstat output",
			wantParsed:     false,
			isDynamicError: false,
		},
		{
			name:           "command with error exit code",
			ibstatCommand:  []string{"sh", "-c", "echo 'some output' >&2; exit 255"},
			expectedError:  errors.New("command exited with error: exit status 255"),
			expectedOutput: "",
			wantParsed:     false,
			isDynamicError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			output, err := GetIbstatOutput(ctx, tt.ibstatCommand)

			if tt.expectedError != nil {
				require.Error(t, err)
				if tt.isDynamicError {
					assert.Contains(t, err.Error(), tt.expectedError.Error(),
						"error message should contain expected content")
				} else {
					assert.True(t, errors.Is(err, tt.expectedError),
						"expected error %v, got %v", tt.expectedError, err)
				}
				if output != nil && tt.expectedOutput != "" {
					assert.Equal(t, tt.expectedOutput, output.Raw, "output content should match exactly")
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, output, "expected non-nil output when no error")

			if tt.expectedOutput != "" {
				assert.Equal(t, tt.expectedOutput, output.Raw, "output content should match exactly")
			}

			if tt.wantParsed {
				assert.NotNil(t, output.Parsed, "expected parsed output but got nil")
			} else {
				assert.Nil(t, output.Parsed, "expected nil parsed output but got parsed data")
			}
		})
	}
}

func TestGetIbstatOutputFailWithPartialOutput(t *testing.T) {
	command := "cat " + filepath.Join("testdata", "ibstat.39.0.a100.failed.ibpanic.exit-255.0") + " && exit 255"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	output, err := GetIbstatOutput(ctx, []string{command})
	require.Equal(t, "command exited with error: exit status 255", err.Error())
	require.NotNil(t, output)
	require.Equal(t, len(output.Parsed), 8)
}

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
	assert.NoError(t, err)
	assert.NotNil(t, parsed)

	// Verify specific fields
	assert.Len(t, parsed, 1)
	if len(parsed) > 0 {
		card := parsed[0]
		assert.Equal(t, "mlx5_0", card.Device)
		assert.Equal(t, "MT4129", card.Type)
		assert.Equal(t, "28.39.1002", card.FirmwareVersion)
		assert.Equal(t, "Active", card.Port1.State)
		assert.Equal(t, "LinkUp", card.Port1.PhysicalState)
		assert.Equal(t, 400, card.Port1.Rate)
		assert.Equal(t, "Ethernet", card.Port1.LinkLayer)
	}
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

func TestValidateIbstatOutput(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedError error
	}{
		{
			name: "both state down and physical state disabled",
			input: `
CA 'mlx5_0'
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
		Link layer: Ethernet
`,
			expectedError: ErrIbstatOutputBrokenStateDown,
		},
		{
			name: "state down but physical state up",
			input: `
CA 'mlx5_0'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300bb3514
	System image GUID: 0xa088c20300bb3514
	Port 1:
		State: Down
		Physical state: LinkUp
		Rate: 40
		Link layer: Ethernet
`,
			expectedError: ErrIbstatOutputBrokenStateDown,
		},
		{
			name: "state active but physical state disabled",
			input: `
CA 'mlx5_0'
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
		Link layer: Ethernet
`,
			expectedError: ErrIbstatOutputBrokenPhysicalDisabled,
		},
		{
			name: "state and physical state both good",
			input: `
CA 'mlx5_0'
	CA type: MT4129
	Number of ports: 1
	Firmware version: 28.39.1002
	Hardware version: 0
	Node GUID: 0xa088c20300bb3514
	System image GUID: 0xa088c20300bb3514
	Port 1:
		State: Active
		Physical state: LinkUp
		Rate: 40
		Link layer: Ethernet
`,
			expectedError: nil,
		},
		{
			name:          "empty input",
			input:         "",
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIbstatOutput(tt.input)
			if tt.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
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
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_2",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_3",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
			},
			atLeastPorts: 4,
			atLeastRate:  200,
			wantErr:      nil,
		},
		{
			name: "all ports active with higher rate than required",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
			},
			atLeastPorts: 2,
			atLeastRate:  200,
			wantErr:      nil,
		},
		{
			name: "all ports disabled but with matching rate",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
			},
			atLeastPorts: 2,
			atLeastRate:  200,
			wantErr:      errors.New("only 0 ports (>= 200 Gb/s) are active, expect at least 2; 2 device(s) found Disabled (mlx5_0, mlx5_1)"),
		},
		{
			name: "some ports down",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
				{
					Device: "mlx5_2",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_3",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
			},
			atLeastPorts: 4,
			atLeastRate:  200,
			wantErr:      errors.New("only 2 ports (>= 200 Gb/s) are active, expect at least 4; 2 device(s) found Disabled (mlx5_1, mlx5_3)"),
		},
		{
			name: "wrong rate",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100},
				},
			},
			atLeastPorts: 2,
			atLeastRate:  200,
			wantErr:      errors.New("only 0 ports (>= 200 Gb/s) are active, expect at least 2"),
		},
		{
			name: "mixed rates with lower threshold",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_2",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 400},
				},
			},
			atLeastPorts: 2,
			atLeastRate:  200,
			wantErr:      nil,
		},
		{
			name: "mixed states with empty expected state matches all",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
				{
					Device: "mlx5_2",
					Port1:  IBStatPort{State: "Init", PhysicalState: "LinkUp", Rate: 200},
				},
			},
			atLeastPorts: 3,
			atLeastRate:  200,
			wantErr:      errors.New("only 2 ports (>= 200 Gb/s) are active, expect at least 3; 1 device(s) found Disabled (mlx5_1)"),
		},
		{
			name: "mixed states with empty expected state matches all and with polling state",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
				{
					Device: "mlx5_2",
					Port1:  IBStatPort{State: "Init", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_3",
					Port1:  IBStatPort{State: "Init", PhysicalState: "Polling", Rate: 200},
				},
			},
			atLeastPorts: 3,
			atLeastRate:  200,
			wantErr:      errors.New("only 2 ports (>= 200 Gb/s) are active, expect at least 3; 1 device(s) found Disabled (mlx5_1); 1 device(s) found Polling (mlx5_3)"),
		},
		{
			name: "mixed states with wrong rate",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 100},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Down", PhysicalState: "LinkUp", Rate: 100},
				},
				{
					Device: "mlx5_2",
					Port1:  IBStatPort{State: "Init", PhysicalState: "LinkUp", Rate: 100},
				},
			},
			atLeastPorts: 3,
			atLeastRate:  200,
			wantErr:      errors.New("only 0 ports (>= 200 Gb/s) are active, expect at least 3"),
		},
		{
			name:         "empty cards",
			cards:        IBStatCards{},
			atLeastPorts: 2,
			atLeastRate:  200,
			wantErr:      errors.New("only 0 ports (>= 200 Gb/s) are active, expect at least 2"),
		},
		{
			name: "some ports disabled but with high enough rate",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
				{
					Device: "mlx5_2",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_3",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
			},
			atLeastPorts: 4,
			atLeastRate:  200,
			wantErr:      errors.New("only 2 ports (>= 200 Gb/s) are active, expect at least 4; 2 device(s) found Disabled (mlx5_1, mlx5_3)"),
		},
		{
			name: "some ports disabled but with high enough rate but missing ports/rates",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
				{
					Device: "mlx5_2",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
				{
					Device: "mlx5_3",
					Port1:  IBStatPort{State: "Down", PhysicalState: "Disabled", Rate: 200},
				},
			},
			atLeastPorts: 0,
			atLeastRate:  0,
			wantErr:      nil,
		},
		{
			name: "zero required ports",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 200},
				},
			},
			atLeastPorts: 0,
			atLeastRate:  200,
			wantErr:      nil,
		},
		{
			name: "zero required rate",
			cards: IBStatCards{
				{
					Device: "mlx5_0",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 0},
				},
				{
					Device: "mlx5_1",
					Port1:  IBStatPort{State: "Active", PhysicalState: "LinkUp", Rate: 0},
				},
			},
			atLeastPorts: 2,
			atLeastRate:  0,
			wantErr:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := tt.cards.CheckPortsAndRate(tt.atLeastPorts, tt.atLeastRate)

			if tt.wantErr == nil {
				if gotErr != nil {
					t.Errorf("CheckPortsAndRate() expected no error, got %v", gotErr)
				}
			} else if gotErr == nil || gotErr.Error() != tt.wantErr.Error() {
				t.Errorf("CheckPortsAndRate() expected error:\n%v\n\nwant\n%v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestParseIBStatEmptyInput(t *testing.T) {
	t.Parallel()

	_, err := ParseIBStat("")
	assert.ErrorIs(t, err, ErrIbstatOutputEmpty, "Expected ErrIbstatOutputEmpty for empty input")
}

func TestParseIBStatNoCardFound(t *testing.T) {
	t.Parallel()

	input := `
	Some random text that doesn't contain any CA entries
	More random text
	`
	_, err := ParseIBStat(input)
	assert.ErrorIs(t, err, ErrIbstatOutputNoCardFound, "Expected ErrIbstatOutputNoCardFound when no cards found")
}

func TestCheckInfiniband(t *testing.T) {
	tests := []struct {
		name           string
		ibstatCommand  string
		threshold      ExpectedPortStates
		expectedError  error
		mockOutputFile string
		wantErr        bool
	}{
		{
			name:           "all ports active and meeting threshold",
			ibstatCommand:  "cat testdata/ibstat.47.0.a100.all.active.0",
			threshold:      ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 200},
			expectedError:  nil,
			mockOutputFile: "",
			wantErr:        false,
		},
		{
			name:           "insufficient port count",
			ibstatCommand:  "cat testdata/ibstat.47.0.a100.all.active.0",
			threshold:      ExpectedPortStates{AtLeastPorts: 10, AtLeastRate: 200},
			expectedError:  errors.New("only 9 ports (>= 200 Gb/s) are active, expect at least 10"),
			mockOutputFile: "",
			wantErr:        true,
		},
		{
			name:           "insufficient rate",
			ibstatCommand:  "cat testdata/ibstat.47.0.a100.all.active.0",
			threshold:      ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 400},
			expectedError:  errors.New("only 0 ports (>= 400 Gb/s) are active, expect at least 8"),
			mockOutputFile: "",
			wantErr:        true,
		},
		{
			name:           "command not found",
			ibstatCommand:  "nonexistent_command",
			threshold:      ExpectedPortStates{AtLeastPorts: 8, AtLeastRate: 200},
			expectedError:  ErrNoIbstatCommand,
			mockOutputFile: "",
			wantErr:        true,
		},
		{
			name:           "zero threshold",
			ibstatCommand:  "cat testdata/ibstat.47.0.a100.all.active.0",
			threshold:      ExpectedPortStates{AtLeastPorts: 0, AtLeastRate: 0},
			expectedError:  nil,
			mockOutputFile: "",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := CheckInfiniband(ctx, tt.ibstatCommand, tt.threshold)

			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedError != nil && !errors.Is(err, tt.expectedError) {
					if !strings.Contains(err.Error(), tt.expectedError.Error()) {
						t.Errorf("expected error containing %v, got %v", tt.expectedError, err)
					}
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIBStatCardsIBPorts(t *testing.T) {
	cards := IBStatCards{
		{
			Device: "mlx5_0",
			Port1: IBStatPort{
				State:         "Active",
				PhysicalState: "LinkUp",
				Rate:          200,
			},
		},
		{
			Device: "mlx5_1",
			Port1: IBStatPort{
				State:         "Down",
				PhysicalState: "Disabled",
				Rate:          400,
			},
		},
	}

	ports := cards.IBPorts()

	require.Equal(t, 2, len(ports))

	// Check first port
	require.Equal(t, "mlx5_0", ports[0].Device)
	require.Equal(t, "Active", ports[0].State)
	require.Equal(t, "LinkUp", ports[0].PhysicalState)
	require.Equal(t, 200, ports[0].Rate)

	// Check second port
	require.Equal(t, "mlx5_1", ports[1].Device)
	require.Equal(t, "Down", ports[1].State)
	require.Equal(t, "Disabled", ports[1].PhysicalState)
	require.Equal(t, 400, ports[1].Rate)
}
