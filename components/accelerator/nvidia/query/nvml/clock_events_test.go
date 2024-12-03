package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

func TestGetClockEventReasons(t *testing.T) {
	tests := []struct {
		name             string
		reasons          uint64
		wantHWSlowdown   []string
		wantOtherReasons []string
	}{
		{
			name:             "no reasons",
			reasons:          0x0000000000000000,
			wantHWSlowdown:   []string{},
			wantOtherReasons: []string{},
		},
		{
			name:    "single hw slowdown reason",
			reasons: reasonHWSlowdown,
			wantHWSlowdown: []string{
				"HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query)",
			},
			wantOtherReasons: []string{},
		},
		{
			name:           "single other reason",
			reasons:        reasonGPUIdle,
			wantHWSlowdown: []string{},
			wantOtherReasons: []string{
				"GPU is idle and clocks are dropping to Idle state",
			},
		},
		{
			name:    "multiple hw slowdown reasons",
			reasons: reasonHWSlowdown | reasonHWSlowdownThermal | reasonHWSlowdownPowerBrake,
			wantHWSlowdown: []string{
				"HW Power Brake Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (External Power Brake Assertion being triggered) ('HW Power Brake Slowdown' in nvidia-smi --query)",
				"HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query)",
				"HW Thermal Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (temperature being too high) ('HW Thermal Slowdown' in nvidia-smi --query)",
			},
			wantOtherReasons: []string{},
		},
		{
			name:           "multiple other reasons",
			reasons:        reasonGPUIdle | reasonApplicationsClocksSetting | reasonSWPowerCap,
			wantHWSlowdown: []string{},
			wantOtherReasons: []string{
				"Clocks have been optimized to not exceed currently set power limits ('SW Power Cap: Active' in nvidia-smi --query)",
				"GPU clocks are limited by current setting of applications clocks",
				"GPU is idle and clocks are dropping to Idle state",
			},
		},
		{
			name:    "mixed hw slowdown and other reasons",
			reasons: reasonHWSlowdown | reasonGPUIdle | reasonHWSlowdownThermal | reasonApplicationsClocksSetting,
			wantHWSlowdown: []string{
				"HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query)",
				"HW Thermal Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (temperature being too high) ('HW Thermal Slowdown' in nvidia-smi --query)",
			},
			wantOtherReasons: []string{
				"GPU clocks are limited by current setting of applications clocks",
				"GPU is idle and clocks are dropping to Idle state",
			},
		},
		{
			name:    "all reasons",
			reasons: 0xFFFFFFFFFFFFFFFF,
			wantHWSlowdown: []string{
				"HW Power Brake Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (External Power Brake Assertion being triggered) ('HW Power Brake Slowdown' in nvidia-smi --query)",
				"HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query)",
				"HW Thermal Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (temperature being too high) ('HW Thermal Slowdown' in nvidia-smi --query)",
			},
			wantOtherReasons: []string{
				"Clocks have been optimized to not exceed currently set power limits ('SW Power Cap: Active' in nvidia-smi --query)",
				"GPU clocks are limited by current setting of Display clocks",
				"GPU clocks are limited by current setting of applications clocks",
				"GPU is idle and clocks are dropping to Idle state",
				"GPU is part of a Sync boost group to maximize performance per watt",
				"SW Thermal Slowdown is active to keep GPU and memory temperatures within operating limits",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHWSlowdown, gotOtherReasons := getClockEventReasons(tt.reasons)

			// Check HW slowdown reasons
			if len(gotHWSlowdown) != len(tt.wantHWSlowdown) {
				t.Errorf("getClockEventReasons() hwSlowdown length = %v, want %v", len(gotHWSlowdown), len(tt.wantHWSlowdown))
			}
			for i, reason := range gotHWSlowdown {
				if i >= len(tt.wantHWSlowdown) {
					t.Errorf("getClockEventReasons() unexpected hwSlowdown reason: %v", reason)
					continue
				}
				if reason != tt.wantHWSlowdown[i] {
					t.Errorf("getClockEventReasons() hwSlowdown[%d] = %v, want %v", i, reason, tt.wantHWSlowdown[i])
				}
			}

			// Check other reasons
			if len(gotOtherReasons) != len(tt.wantOtherReasons) {
				t.Errorf("getClockEventReasons() otherReasons length = %v, want %v", len(gotOtherReasons), len(tt.wantOtherReasons))
			}
			for i, reason := range gotOtherReasons {
				if i >= len(tt.wantOtherReasons) {
					t.Errorf("getClockEventReasons() unexpected other reason: %v", reason)
					continue
				}
				if reason != tt.wantOtherReasons[i] {
					t.Errorf("getClockEventReasons() otherReasons[%d] = %v, want %v", i, reason, tt.wantOtherReasons[i])
				}
			}
		})
	}
}

func TestGetClockEvents(t *testing.T) {
	testCases := []struct {
		name           string
		uuid           string
		mockReasons    uint64
		mockReturn     nvml.Return
		expectedError  bool
		expectedEvents ClockEvents
	}{
		{
			name:        "success with no events",
			uuid:        "GPU-1234",
			mockReasons: 0,
			mockReturn:  nvml.SUCCESS,
			expectedEvents: ClockEvents{
				UUID:           "GPU-1234",
				ReasonsBitmask: 0,
			},
		},
		{
			name:        "success with HW slowdown",
			uuid:        "GPU-5678",
			mockReasons: reasonHWSlowdown,
			mockReturn:  nvml.SUCCESS,
			expectedEvents: ClockEvents{
				UUID:           "GPU-5678",
				ReasonsBitmask: reasonHWSlowdown,
				HWSlowdown:     true,
				HWSlowdownReasons: []string{
					"GPU-5678: HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query) (nvml)",
				},
			},
		},
		{
			name:        "success with thermal slowdown",
			uuid:        "GPU-ABCD",
			mockReasons: reasonHWSlowdownThermal,
			mockReturn:  nvml.SUCCESS,
			expectedEvents: ClockEvents{
				UUID:              "GPU-ABCD",
				ReasonsBitmask:    reasonHWSlowdownThermal,
				HWSlowdownThermal: true,
				HWSlowdownReasons: []string{
					"GPU-ABCD: HW Thermal Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (temperature being too high) ('HW Thermal Slowdown' in nvidia-smi --query) (nvml)",
				},
			},
		},
		{
			name:          "nvml error",
			uuid:          "GPU-ERROR",
			mockReasons:   0,
			mockReturn:    nvml.ERROR_UNKNOWN,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := &mock.Device{
				GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
					return tc.mockReasons, tc.mockReturn
				},
			}
			events, err := GetClockEvents(tc.uuid, createMockDevice(mockDevice))

			if tc.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if events.UUID != tc.expectedEvents.UUID {
				t.Errorf("UUID mismatch: got %s, want %s",
					events.UUID, tc.expectedEvents.UUID)
			}
			if events.ReasonsBitmask != tc.expectedEvents.ReasonsBitmask {
				t.Errorf("ReasonsBitmask mismatch: got %d, want %d",
					events.ReasonsBitmask, tc.expectedEvents.ReasonsBitmask)
			}
			if events.HWSlowdown != tc.expectedEvents.HWSlowdown {
				t.Errorf("HWSlowdown mismatch: got %v, want %v",
					events.HWSlowdown, tc.expectedEvents.HWSlowdown)
			}
			if events.HWSlowdownThermal != tc.expectedEvents.HWSlowdownThermal {
				t.Errorf("HWSlowdownThermal mismatch: got %v, want %v",
					events.HWSlowdownThermal, tc.expectedEvents.HWSlowdownThermal)
			}

			if len(events.HWSlowdownReasons) != len(tc.expectedEvents.HWSlowdownReasons) {
				t.Errorf("HWSlowdownReasons length mismatch: got %d, want %d",
					len(events.HWSlowdownReasons), len(tc.expectedEvents.HWSlowdownReasons))
			} else {
				for i, reason := range events.HWSlowdownReasons {
					if reason != tc.expectedEvents.HWSlowdownReasons[i] {
						t.Errorf("HWSlowdownReason mismatch at index %d: got %s, want %s",
							i, reason, tc.expectedEvents.HWSlowdownReasons[i])
					}
				}
			}
		})
	}
}

type mockDevice struct {
	*mock.Device
}

func (d *mockDevice) GetArchitectureAsString() (string, error) {
	return "", nil
}
func (d *mockDevice) GetBrandAsString() (string, error) {
	return "", nil
}
func (d *mockDevice) GetCudaComputeCapabilityAsString() (string, error) {
	return "", nil
}
func (d *mockDevice) GetMigDevices() ([]device.MigDevice, error) {
	return nil, nil
}
func (d *mockDevice) GetMigProfiles() ([]device.MigProfile, error) {
	return nil, nil
}
func (d *mockDevice) GetPCIBusID() (string, error)                                { return "", nil }
func (d *mockDevice) IsFabricAttached() (bool, error)                             { return false, nil }
func (d *mockDevice) IsMigCapable() (bool, error)                                 { return false, nil }
func (d *mockDevice) IsMigEnabled() (bool, error)                                 { return false, nil }
func (d *mockDevice) VisitMigDevices(func(j int, m device.MigDevice) error) error { return nil }
func (d *mockDevice) VisitMigProfiles(func(p device.MigProfile) error) error      { return nil }

func createMockDevice(m *mock.Device) device.Device {
	return &mockDevice{Device: m}
}
