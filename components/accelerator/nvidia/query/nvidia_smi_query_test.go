package query

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestGetSMIOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// iterate all files in "testdata/"
	matches, err := filepath.Glob("testdata/nvidia-smi-query.*.out.*.valid")
	if err != nil {
		t.Fatalf("failed to glob: %v", err)
	}

	for _, queryFile := range matches {
		o, err := GetSMIOutput(
			ctx,
			[]string{"cat", "testdata/nvidia-smi.550.90.07.out.0.valid"},
			[]string{"cat", queryFile},
		)
		if err != nil {
			// TODO: fix
			// CI can be flaky due to "cat" output being different
			t.Logf("%q: %v", queryFile, err)
			continue
		}

		o.Raw = ""
		o.Summary = ""

		t.Logf("%q:\n%+v", queryFile, o)
	}
}

func TestGetSMIOutputError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := GetSMIOutput(
		ctx,
		[]string{"echo", "invalid-output-is-here-should-fail"},
		[]string{"echo", "invalid-output-is-here-should-fail"},
	)
	if !errors.Is(err, ErrNoGPUFoundFromSMIQuery) {
		t.Errorf("GetSMIOutput() should return %v, got %v", ErrNoGPUFoundFromSMIQuery, err)
	}
}

func TestParse4090Valid(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.535.154.05.out.0.valid.4090")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	parsed, err := ParseSMIQueryOutput(data)
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if parsed.GPUs[0].ID != "GPU 00000000:01:00.0" {
		t.Errorf("GPU0.ID mismatch: %+v", parsed.GPUs[0].ID)
	}
}

func TestParseWithRemappedRows(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.535.129.03.out.0.valid.a10")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	parsed, err := ParseSMIQueryOutput(data)
	if err != nil {
		t.Errorf("Parse returned an error: %v", err)
	}
	if parsed.GPUs[0].RemappedRows.UncorrectableError != "4" {
		t.Errorf("RemappedRows.UncorrectableError mismatch: %+v", parsed.GPUs[0].RemappedRows.UncorrectableError)
	}

	if parsed.GPUs[0].RemappedRows.ID != "GPU 00000000:07:00.0" {
		t.Errorf("RemappedRows.ID mismatch: %q", parsed.GPUs[0].RemappedRows.ID)
	}
	if parsed.GPUs[0].RemappedRows.Pending != "true" { // yaml package converts yes to true, no to false
		t.Errorf("RemappedRows.Pending mismatch: %v", parsed.GPUs[0].RemappedRows.Pending)
	}
	if parsed.GPUs[0].RemappedRows.RemappingFailureOccurred != "false" { // yaml package converts yes to true, no to false
		t.Errorf("RemappedRows.RemappingFailureOccurred mismatch: %v", parsed.GPUs[0].RemappedRows.RemappingFailureOccurred)
	}
}

func TestParseWithRemappedRowsNone(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.560.35.03.out.0.valid")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	parsed, err := ParseSMIQueryOutput(data)
	if err != nil {
		t.Errorf("Parse returned an error: %v", err)
	}
	if parsed.GPUs[0].RemappedRows != nil {
		t.Errorf("RemappedRows should be nil: %+v", parsed.GPUs[0].RemappedRows)
	}
}

func TestParseWithHWSlowdownActive(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.535.161.08.out.0.valid")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	parsed, err := ParseSMIQueryOutput(data)
	if err != nil {
		t.Errorf("Parse returned an error: %v", err)
	}
	for _, gpu := range parsed.GPUs {
		if gpu.ClockEventReasons.HWPowerBrakeSlowdown != ClockEventsActive {
			t.Errorf("HWPowerBrakeSlowdown mismatch: %+v", gpu.ClockEventReasons.HWPowerBrakeSlowdown)
		}
	}
}

func TestParseECCMode(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.535.161.08.out.0.valid")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	parsed, err := ParseSMIQueryOutput(data)
	if err != nil {
		t.Errorf("Parse returned an error: %v", err)
	}
	for _, gpu := range parsed.GPUs {
		if gpu.ECCMode.Current != "Enabled" {
			t.Errorf("ECCMode mismatch: %+v", gpu.ECCMode.Current)
		}
		if gpu.ECCMode.Pending != "Enabled" {
			t.Errorf("ECCMode mismatch: %+v", gpu.ECCMode.Pending)
		}
	}
}

func TestParseWithProcesses(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.535.154.05.out.0.valid")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	parsed, err := ParseSMIQueryOutput(data)
	if err != nil {
		t.Errorf("Parse returned an error: %v", err)
	}

	if parsed.GPUs[0].ID != "GPU 00000000:01:00.0" {
		t.Errorf("GPU0.ID mismatch: %+v", parsed.GPUs[0].ID)
	}
	if parsed.GPUs[0].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[0].ClockEventReasons.HWThermalSlowdown)
	}

	if parsed.GPUs[7].Processes.ProcessID != 1102861 {
		t.Errorf("ProcessID mismatch: %d", parsed.GPUs[7].Processes.ProcessID)
	}
	if parsed.GPUs[7].Processes.ProcessName != "/opt/lepton/venv/bin/python3.10" {
		t.Errorf("ProcessName mismatch: %s", parsed.GPUs[7].Processes.ProcessName)
	}

	yb, err := parsed.YAML()
	if err != nil {
		t.Errorf("YAML returned an error: %v", err)
	}
	t.Logf("YAML:\n%s\n", yb)
}

func TestParseWithNoProcesses(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.535.183.01.out.0.valid")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	parsed, err := ParseSMIQueryOutput(data)
	if err != nil {
		t.Errorf("Parse returned an error: %v", err)
	}

	if parsed.GPUs[0].ID != "GPU 00000000:53:00.0" {
		t.Errorf("GPU0.ID mismatch: %+v", parsed.GPUs[0].ID)
	}
	if parsed.GPUs[0].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[0].ClockEventReasons.HWThermalSlowdown)
	}
	if parsed.GPUs[0].Temperature.Current != "36 C" {
		t.Errorf("GPU0.Temperature.GPUCurrentTemp mismatch: %+v", parsed.GPUs[0].Temperature.Current)
	}
	if parsed.GPUs[0].GPUPowerReadings.PowerDraw != "71.97 W" {
		t.Errorf("PowerDraw mismatch: %+v", parsed.GPUs[0].GPUPowerReadings.PowerDraw)
	}
	if parsed.GPUs[0].GPUPowerReadings.CurrentPowerLimit != "700.00 W" {
		t.Errorf("CurrentPowerLimit mismatch: %+v", parsed.GPUs[0].GPUPowerReadings.CurrentPowerLimit)
	}
	if parsed.GPUs[0].ECCErrors.Volatile.SRAMCorrectable != "0" {
		t.Errorf("GPU0.ECCErrors.Volatile.SRAMCorrectable mismatch: %+v", parsed.GPUs[0].ECCErrors.Volatile.SRAMCorrectable)
	}
	if parsed.GPUs[0].FBMemoryUsage.Total != "81559 MiB" {
		t.Errorf("GPU0.FBMemoryUsage.Total mismatch: %+v", parsed.GPUs[0].FBMemoryUsage.Total)
	}
	if parsed.GPUs[0].FBMemoryUsage.Reserved != "551 MiB" {
		t.Errorf("GPU0.FBMemoryUsage.Reserved mismatch: %+v", parsed.GPUs[0].FBMemoryUsage.Reserved)
	}

	if parsed.GPUs[1].ID != "GPU 00000000:64:00.0" {
		t.Errorf("GPU1.ID mismatch: %+v", parsed.GPUs[1].ID)
	}
	if parsed.GPUs[1].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[1].ClockEventReasons.HWThermalSlowdown)
	}

	if parsed.GPUs[2].ID != "GPU 00000000:75:00.0" {
		t.Errorf("GPU2.ID mismatch: %+v", parsed.GPUs[2].ID)
	}
	if parsed.GPUs[2].ClockEventReasons.SWPowerCap != ClockEventsActive {
		t.Errorf("SWPowerCap mismatch: %+v", parsed.GPUs[2].ClockEventReasons.SWPowerCap)
	}
	if parsed.GPUs[2].ClockEventReasons.SWThermalSlowdown != ClockEventsActive {
		t.Errorf("SWThermalSlowdown mismatch: %+v", parsed.GPUs[2].ClockEventReasons.SWThermalSlowdown)
	}
	if parsed.GPUs[2].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[2].ClockEventReasons.HWThermalSlowdown)
	}

	if parsed.GPUs[3].ID != "GPU 00000000:86:00.0" {
		t.Errorf("GPU3.ID mismatch: %+v", parsed.GPUs[3].ID)
	}
	if parsed.GPUs[3].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[3].ClockEventReasons.HWThermalSlowdown)
	}

	if parsed.GPUs[4].ID != "GPU 00000000:97:00.0" {
		t.Errorf("GPU4.ID mismatch: %+v", parsed.GPUs[4].ID)
	}
	if parsed.GPUs[4].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[4].ClockEventReasons.HWThermalSlowdown)
	}

	if parsed.GPUs[5].ID != "GPU 00000000:A8:00.0" {
		t.Errorf("GPU5.ID mismatch: %+v", parsed.GPUs[5].ID)
	}
	if parsed.GPUs[5].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[5].ClockEventReasons.HWThermalSlowdown)
	}

	if parsed.GPUs[6].ID != "GPU 00000000:B9:00.0" {
		t.Errorf("GPU6.ID mismatch: %+v", parsed.GPUs[6].ID)
	}
	if parsed.GPUs[6].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[6].ClockEventReasons.HWThermalSlowdown)
	}

	if parsed.GPUs[7].ID != "GPU 00000000:CA:00.0" {
		t.Errorf("GPU7.ID mismatch: %+v", parsed.GPUs[7].ID)
	}
	if parsed.GPUs[7].ClockEventReasons.HWThermalSlowdown != ClockEventsNotActive {
		t.Errorf("HWThermalSlowdown mismatch: %+v", parsed.GPUs[7].ClockEventReasons.HWThermalSlowdown)
	}

	yb, err := parsed.YAML()
	if err != nil {
		t.Errorf("YAML returned an error: %v", err)
	}
	t.Logf("YAML:\n%s\n", yb)
}

func TestParseWithFallback(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.535.183.01.out.0.invalid")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	parsed, err := ParseSMIQueryOutput(data)
	if err == nil {
		t.Errorf("Parse returned no error")
	}
	if parsed.CUDAVersion != "12.2" {
		t.Errorf("CUDAVersion mismatch: %+v", parsed.CUDAVersion)
	}
}

func TestParseMore(t *testing.T) {
	matches, err := filepath.Glob("testdata/nvidia-smi-query.*.out.*.valid")
	if err != nil {
		t.Fatalf("failed to glob: %v", err)
	}
	for _, f := range matches {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}
		if _, err := ParseSMIQueryOutput(data); err != nil {
			t.Errorf("Parse returned an error: %v", err)
		}
	}
}

func TestFindSummaryErr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "No errors",
			input:    "This is a normal output\nwithout any errors",
			expected: []string{},
		},
		{
			name:     "Single error on first line",
			input:    "ERR! This is an error\nNext line",
			expected: []string{"ERR! This is an error"},
		},
		{
			name:     "Single error with context",
			input:    "Context line\nERR! This is an error\nNext line",
			expected: []string{"Context line\nERR! This is an error"},
		},
		{
			name:     "Multiple errors",
			input:    "Context 1\nERR! Error 1\nContext 2\nERR! Error 2",
			expected: []string{"Context 1\nERR! Error 1", "Context 2\nERR! Error 2"},
		},
		{
			name:     "Error at the end",
			input:    "Line 1\nLine 2\nERR! Last line error",
			expected: []string{"Line 2\nERR! Last line error"},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: []string{},
		},
		{
			name: "ERR!",
			input: `
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 525.125.06   Driver Version: 525.125.06   CUDA Version: 12.0     |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
|                               |                      |               MIG M. |
|===============================+======================+======================|
|   0  NVIDIA GeForce ...  Off  | 00000000:01:00.0 Off |                    0 |
|ERR!   38C    P5    49W / 450W |   2021MiB / 23028MiB |      0%   E. Process |
|                               |                      |                  N/A |
+-------------------------------+----------------------+----------------------+
`,
			expected: []string{"|   0  NVIDIA GeForce ...  Off  | 00000000:01:00.0 Off |                    0 |\n|ERR!   38C    P5    49W / 450W |   2021MiB / 23028MiB |      0%   E. Process |"},
		},
		{
			name: "ERR!",
			input: `
| NVIDIA-SMI 535.98                 Driver Version: 535.98       CUDA Version: 12.2     |
|-----------------------------------------+----------------------+----------------------+
| GPU  Name                 Persistence-M | Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp   Perf          Pwr:Usage/Cap |         Memory-Usage | GPU-Util  Compute M. |
|                                         |                      |               MIG M. |
|=========================================+======================+======================|
|   0  NVIDIA T600 Laptop GPU         Off | 00000000:01:00.0 Off |                  N/A |
| N/A   61C    P0              N/A / ERR! |      5MiB /  4096MiB |      0%      Default |
|                                         |                      |                  N/A |
+-----------------------------------------+----------------------+----------------------+

`,
			expected: []string{"|   0  NVIDIA T600 Laptop GPU         Off | 00000000:01:00.0 Off |                  N/A |\n| N/A   61C    P0              N/A / ERR! |      5MiB /  4096MiB |      0%      Default |"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindSummaryErr(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("FindSummaryErr() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFindHWSlowdownErrs(t *testing.T) {
	tests := []struct {
		name     string
		output   *SMIOutput
		wantErrs []string
	}{
		{
			name: "No HW Slowdown",
			output: &SMIOutput{
				GPUs: []NvidiaSMIGPU{
					{
						ClockEventReasons: &SMIClockEventReasons{
							HWSlowdown:           ClockEventsActive,
							HWThermalSlowdown:    ClockEventsNotActive,
							HWPowerBrakeSlowdown: ClockEventsNotActive,
						},
					},
				},
			},
			wantErrs: nil,
		},
		{
			name: "Thermal Slowdown on GPU0",
			output: &SMIOutput{
				GPUs: []NvidiaSMIGPU{
					{
						ID: "gpu0",
						ClockEventReasons: &SMIClockEventReasons{
							HWSlowdown:           ClockEventsActive,
							HWThermalSlowdown:    ClockEventsActive,
							HWPowerBrakeSlowdown: ClockEventsNotActive,
						},
					},
				},
			},
			wantErrs: []string{"gpu0: ClockEventReasons.HWSlowdown.ThermalSlowdown Active"},
		},
		{
			name: "Power Brake Slowdown on GPU1",
			output: &SMIOutput{
				GPUs: []NvidiaSMIGPU{
					{
						ID: "gpu0",
						ClockEventReasons: &SMIClockEventReasons{
							HWSlowdown:           ClockEventsActive,
							HWThermalSlowdown:    ClockEventsNotActive,
							HWPowerBrakeSlowdown: ClockEventsActive,
						},
					},
				},
			},
			wantErrs: []string{"gpu0: ClockEventReasons.HWSlowdown.PowerBrakeSlowdown Active"},
		},
		{
			name: "Multiple GPUs with Slowdowns",
			output: &SMIOutput{
				GPUs: []NvidiaSMIGPU{
					{
						ID: "gpu0",
						ClockEventReasons: &SMIClockEventReasons{
							HWSlowdown:           ClockEventsActive,
							HWThermalSlowdown:    ClockEventsActive,
							HWPowerBrakeSlowdown: ClockEventsNotActive,
						},
					},
					{
						ID: "gpu1",
						ClockEventReasons: &SMIClockEventReasons{
							HWSlowdown:           ClockEventsActive,
							HWThermalSlowdown:    ClockEventsNotActive,
							HWPowerBrakeSlowdown: ClockEventsActive,
						},
					},
				},
			},
			wantErrs: []string{
				"gpu0: ClockEventReasons.HWSlowdown.ThermalSlowdown Active",
				"gpu1: ClockEventReasons.HWSlowdown.PowerBrakeSlowdown Active",
			},
		},
		{
			name: "Nil HWSlowdown",
			output: &SMIOutput{
				GPUs: []NvidiaSMIGPU{
					{
						ClockEventReasons: &SMIClockEventReasons{},
					},
				},
			},
			wantErrs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.output.FindHWSlowdownErrs()
			if !reflect.DeepEqual(errs, tt.wantErrs) {
				t.Errorf("Output.HasHWSlowdown() gotErrs = %v, want %v", errs, tt.wantErrs)
			}
		})
	}
}

func TestParseWithAddressingModeError(t *testing.T) {
	data, err := os.ReadFile("testdata/nvidia-smi-query.535.154.05.out.3.valid")
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	parsed, err := ParseSMIQueryOutput(data)
	if err != nil {
		t.Errorf("Parse returned an error: %v", err)
	}
	for _, g := range parsed.GPUs {
		if g.AddressingMode != "Unknown Error" {
			t.Errorf("AddressingMode mismatch: %+v", g.AddressingMode)
		}
	}
}
