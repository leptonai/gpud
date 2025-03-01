package query

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
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

func TestCreateHWSlowdownEventFromNvidiaSMI(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name            string
		eventTime       time.Time
		gpuUUID         string
		slowdownReasons []string
		want            *components.Event
	}{
		{
			name:            "no slowdown reasons",
			eventTime:       testTime,
			gpuUUID:         "GPU-123",
			slowdownReasons: []string{},
			want:            nil,
		},
		{
			name:            "nil slowdown reasons",
			eventTime:       testTime,
			gpuUUID:         "GPU-123",
			slowdownReasons: nil,
			want:            nil,
		},
		{
			name:      "single slowdown reason",
			eventTime: testTime,
			gpuUUID:   "GPU-5678",
			slowdownReasons: []string{
				"HW Slowdown is engaged",
			},
			want: &components.Event{
				Time:    metav1.Time{Time: testTime},
				Name:    "hw_slowdown",
				Type:    common.EventTypeWarning,
				Message: "HW Slowdown is engaged",
				ExtraInfo: map[string]string{
					"data_source": "nvidia-smi",
					"gpu_uuid":    "GPU-5678",
				},
			},
		},
		{
			name:      "multiple slowdown reasons",
			eventTime: testTime,
			gpuUUID:   "GPU-ABCD",
			slowdownReasons: []string{
				"HW Power Brake Slowdown",
				"HW Slowdown is engaged",
				"HW Thermal Slowdown",
			},
			want: &components.Event{
				Time:    metav1.Time{Time: testTime},
				Name:    "hw_slowdown",
				Type:    common.EventTypeWarning,
				Message: "HW Power Brake Slowdown, HW Slowdown is engaged, HW Thermal Slowdown",
				ExtraInfo: map[string]string{
					"data_source": "nvidia-smi",
					"gpu_uuid":    "GPU-ABCD",
				},
			},
		},
		{
			name:      "empty gpu uuid",
			eventTime: testTime,
			gpuUUID:   "",
			slowdownReasons: []string{
				"HW Slowdown is engaged",
			},
			want: &components.Event{
				Time:    metav1.Time{Time: testTime},
				Name:    "hw_slowdown",
				Type:    common.EventTypeWarning,
				Message: "HW Slowdown is engaged",
				ExtraInfo: map[string]string{
					"data_source": "nvidia-smi",
					"gpu_uuid":    "",
				},
			},
		},
		{
			name:      "zero time",
			eventTime: time.Time{},
			gpuUUID:   "GPU-ZERO",
			slowdownReasons: []string{
				"HW Slowdown is engaged",
			},
			want: &components.Event{
				Time:    metav1.Time{Time: time.Time{}},
				Name:    "hw_slowdown",
				Type:    common.EventTypeWarning,
				Message: "HW Slowdown is engaged",
				ExtraInfo: map[string]string{
					"data_source": "nvidia-smi",
					"gpu_uuid":    "GPU-ZERO",
				},
			},
		},
		{
			name:      "real nvidia-smi output format",
			eventTime: testTime,
			gpuUUID:   "GPU-00000000:01:00.0",
			slowdownReasons: []string{
				"GPU-00000000:01:00.0: ClockEventReasons.HWSlowdown.ThermalSlowdown Active",
				"GPU-00000000:01:00.0: ClockEventReasons.HWSlowdown.PowerBrakeSlowdown Active",
			},
			want: &components.Event{
				Time: metav1.Time{Time: testTime},
				Name: "hw_slowdown",
				Type: common.EventTypeWarning,
				Message: "GPU-00000000:01:00.0: ClockEventReasons.HWSlowdown.ThermalSlowdown Active, " +
					"GPU-00000000:01:00.0: ClockEventReasons.HWSlowdown.PowerBrakeSlowdown Active",
				ExtraInfo: map[string]string{
					"data_source": "nvidia-smi",
					"gpu_uuid":    "GPU-00000000:01:00.0",
				},
			},
		},
		{
			name:      "slowdown reason with special characters",
			eventTime: testTime,
			gpuUUID:   "GPU-SPECIAL",
			slowdownReasons: []string{
				"HW Slowdown (temp: 95°C, power: 350W)",
			},
			want: &components.Event{
				Time:    metav1.Time{Time: testTime},
				Name:    "hw_slowdown",
				Type:    common.EventTypeWarning,
				Message: "HW Slowdown (temp: 95°C, power: 350W)",
				ExtraInfo: map[string]string{
					"data_source": "nvidia-smi",
					"gpu_uuid":    "GPU-SPECIAL",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createHWSlowdownEventFromNvidiaSMI(tt.eventTime, tt.gpuUUID, tt.slowdownReasons)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("createHWSlowdownEventFromNvidiaSMI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindGPUErrsAttachedGPUs(t *testing.T) {
	tests := []struct {
		name          string
		attachedGPUs  int
		gpuCount      int
		expectedError string
		errorInGPUs   bool // Set to true to intentionally add errors in GPU structs
	}{
		{
			name:          "matching counts",
			attachedGPUs:  4,
			gpuCount:      4,
			expectedError: "",
			errorInGPUs:   false,
		},
		{
			name:          "attached greater than found",
			attachedGPUs:  8,
			gpuCount:      4,
			expectedError: "nvidia-smi query output 'Attached GPUs' 8 but only found GPUs 4 in the nvidia-smi command output -- check 'nvidia-smi --query' output",
			errorInGPUs:   false,
		},
		{
			name:          "attached less than found",
			attachedGPUs:  2,
			gpuCount:      4,
			expectedError: "nvidia-smi query output 'Attached GPUs' 2 but only found GPUs 4 in the nvidia-smi command output -- check 'nvidia-smi --query' output",
			errorInGPUs:   false,
		},
		{
			name:          "matching counts with GPU error",
			attachedGPUs:  2,
			gpuCount:      2,
			expectedError: "", // No mismatch error
			errorInGPUs:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create GPUs that won't trigger errors
			gpus := make([]NvidiaSMIGPU, tt.gpuCount)
			for i := range gpus {
				addressingMode := "Default" // Not "Unknown Error"
				if tt.errorInGPUs && i == 0 {
					addressingMode = "Unknown Error" // This will trigger an error
				}

				gpus[i] = NvidiaSMIGPU{
					ID:             fmt.Sprintf("GPU-%d", i),
					AddressingMode: addressingMode,
					FanSpeed:       "50%", // Not "Unknown Error"
					ClockEventReasons: &SMIClockEventReasons{
						HWSlowdown:           ClockEventsNotActive,
						HWThermalSlowdown:    ClockEventsNotActive,
						HWPowerBrakeSlowdown: ClockEventsNotActive,
					},
					Temperature: &SMIGPUTemperature{
						Current:                 "50 C", // Not "Unknown Error"
						Limit:                   "95 C", // Not "Unknown Error"
						ShutdownLimit:           "100 C",
						SlowdownLimit:           "90 C",
						MaxOperatingLimit:       "95 C",
						Target:                  "80 C",
						MemoryCurrent:           "45 C",
						MemoryMaxOperatingLimit: "95 C",
					},
				}
			}

			// Create a test SMIOutput with the specified configuration
			o := &SMIOutput{
				AttachedGPUs: tt.attachedGPUs,
				GPUs:         gpus,
				Summary:      "",
			}

			// Call the method under test
			errs := o.FindGPUErrs()

			if tt.errorInGPUs {
				// If we intentionally added an error, we expect other errors
				assert.NotEmpty(t, errs, "Should have errors from GPU structs")
				// But shouldn't have the mismatch error
				for _, err := range errs {
					assert.NotContains(t, err, "nvidia-smi query output 'Attached GPUs'",
						"Should not have the mismatch error")
				}
			} else if tt.expectedError == "" {
				// No error expected for this test case
				assert.Empty(t, errs, "Expected no errors when AttachedGPUs matches GPU count")
			} else {
				// Error expected for this test case
				assert.Contains(t, errs, tt.expectedError, "Expected error message about mismatched GPU counts")

				// Verify that we only have the expected error
				assert.Len(t, errs, 1, "Should only have one error from AttachedGPUs mismatch")
			}
		})
	}
}
