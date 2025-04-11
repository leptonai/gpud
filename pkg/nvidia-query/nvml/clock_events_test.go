package nvml

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
	nvml_lib_mock "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib/mock"
	"github.com/leptonai/gpud/pkg/nvidia-query/nvml/testutil"
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
		expectedErrMsg string
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
					"GPU-5678: HW Slowdown is engaged due to high temperature, power brake assertion, or high power draw ('HW Slowdown: Active' in nvidia-smi --query)",
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
					"GPU-ABCD: HW Thermal Slowdown (reducing the core clocks by a factor of 2 or more) is engaged (temperature being too high) ('HW Thermal Slowdown' in nvidia-smi --query)",
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
		{
			name:           "device not ready error",
			uuid:           "GPU-NOT-READY",
			mockReasons:    0,
			mockReturn:     nvml.ERROR_NOT_READY,
			expectedError:  true,
			expectedErrMsg: "device GPU-NOT-READY is not initialized",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDevice := &testutil.MockDevice{
				Device: &mock.Device{
					GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
						return tc.mockReasons, tc.mockReturn
					},
				},
			}

			events, err := GetClockEvents(tc.uuid, mockDevice)

			if tc.expectedError {
				if err == nil {
					t.Error("expected error but got none")
				}
				if tc.expectedErrMsg != "" && !strings.Contains(err.Error(), tc.expectedErrMsg) {
					t.Errorf("error message mismatch: got %v, want to contain %v", err.Error(), tc.expectedErrMsg)
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

func TestClockEventsSupported(t *testing.T) {
	tests := []struct {
		name           string
		mockDevices    []*testutil.MockDevice
		mockDeviceErr  error
		expectedResult bool
		expectError    bool
	}{
		{
			name: "all devices support clock events",
			mockDevices: []*testutil.MockDevice{
				{
					Device: &mock.Device{
						GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
							return 0, nvml.SUCCESS
						},
						GetNameFunc: func() (string, nvml.Return) {
							return "Tesla V100", nvml.SUCCESS
						},
						GetUUIDFunc: func() (string, nvml.Return) {
							return "GPU-1234", nvml.SUCCESS
						},
						GetMinorNumberFunc: func() (int, nvml.Return) {
							return 0, nvml.SUCCESS
						},
					},
				},
				{
					Device: &mock.Device{
						GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
							return 0, nvml.SUCCESS
						},
						GetNameFunc: func() (string, nvml.Return) {
							return "Tesla V100", nvml.SUCCESS
						},
						GetUUIDFunc: func() (string, nvml.Return) {
							return "GPU-5678", nvml.SUCCESS
						},
						GetMinorNumberFunc: func() (int, nvml.Return) {
							return 1, nvml.SUCCESS
						},
					},
				},
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name: "one device supports clock events",
			mockDevices: []*testutil.MockDevice{
				{
					Device: &mock.Device{
						GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
							return 0, nvml.ERROR_NOT_SUPPORTED
						},
						GetNameFunc: func() (string, nvml.Return) {
							return "Tesla V100", nvml.SUCCESS
						},
						GetUUIDFunc: func() (string, nvml.Return) {
							return "GPU-1234", nvml.SUCCESS
						},
						GetMinorNumberFunc: func() (int, nvml.Return) {
							return 0, nvml.SUCCESS
						},
					},
				},
				{
					Device: &mock.Device{
						GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
							return 0, nvml.SUCCESS
						},
						GetNameFunc: func() (string, nvml.Return) {
							return "Tesla V100", nvml.SUCCESS
						},
						GetUUIDFunc: func() (string, nvml.Return) {
							return "GPU-5678", nvml.SUCCESS
						},
						GetMinorNumberFunc: func() (int, nvml.Return) {
							return 1, nvml.SUCCESS
						},
					},
				},
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name: "no devices support clock events",
			mockDevices: []*testutil.MockDevice{
				{
					Device: &mock.Device{
						GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
							return 0, nvml.ERROR_NOT_SUPPORTED
						},
						GetNameFunc: func() (string, nvml.Return) {
							return "Tesla V100", nvml.SUCCESS
						},
						GetUUIDFunc: func() (string, nvml.Return) {
							return "GPU-1234", nvml.SUCCESS
						},
						GetMinorNumberFunc: func() (int, nvml.Return) {
							return 0, nvml.SUCCESS
						},
					},
				},
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name:          "error getting devices",
			mockDeviceErr: fmt.Errorf("failed to get devices"),
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock devices
			mockDevices := make([]device.Device, len(tt.mockDevices))
			for i, d := range tt.mockDevices {
				mockDevices[i] = d
			}

			// Mock NVML
			mockNVML := &mock.Interface{
				InitFunc: func() nvml.Return {
					return nvml.SUCCESS
				},
				DeviceGetCountFunc: func() (int, nvml.Return) {
					if tt.mockDeviceErr != nil {
						return 0, nvml.ERROR_UNKNOWN
					}
					return len(mockDevices), nvml.SUCCESS
				},
				DeviceGetHandleByIndexFunc: func(idx int) (nvml.Device, nvml.Return) {
					if tt.mockDeviceErr != nil {
						return nil, nvml.ERROR_UNKNOWN
					}
					if idx < 0 || idx >= len(tt.mockDevices) {
						return nil, nvml.ERROR_INVALID_ARGUMENT
					}
					return tt.mockDevices[idx].Device, nvml.SUCCESS
				},
			}

			err := os.Setenv(nvml_lib.EnvMockAllSuccess, "true")
			if err != nil {
				t.Fatalf("failed to set mock NVML environment: %v", err)
			}
			defer os.Unsetenv(nvml_lib.EnvMockAllSuccess)

			// Replace the mock instance
			originalMockInstance := nvml_lib_mock.AllSuccessInterface
			nvml_lib_mock.AllSuccessInterface = mockNVML
			defer func() { nvml_lib_mock.AllSuccessInterface = originalMockInstance }()

			result, err := ClockEventsSupported()
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expectedResult {
				t.Errorf("ClockEventsSupported() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestClockEventsSupportedByDevice(t *testing.T) {
	tests := []struct {
		name           string
		mockDevice     *testutil.MockDevice
		expectedResult bool
		expectError    bool
	}{
		{
			name: "device supports clock events",
			mockDevice: &testutil.MockDevice{
				Device: &mock.Device{
					GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
						return 0, nvml.SUCCESS
					},
				},
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name: "device does not support clock events",
			mockDevice: &testutil.MockDevice{
				Device: &mock.Device{
					GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
						return 0, nvml.ERROR_NOT_SUPPORTED
					},
				},
			},
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "device returns error",
			mockDevice: &testutil.MockDevice{
				Device: &mock.Device{
					GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
						return 0, nvml.ERROR_UNKNOWN
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ClockEventsSupportedByDevice(tt.mockDevice)
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expectedResult {
				t.Errorf("ClockEventsSupportedByDevice() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestClockEventsJSONAndYAML(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		clockEvents *ClockEvents
		wantJSON    string
		wantYAML    string
	}{
		{
			name: "valid clock events",
			clockEvents: &ClockEvents{
				Time:              metav1.Time{Time: testTime},
				UUID:              "GPU-123",
				ReasonsBitmask:    reasonHWSlowdown,
				HWSlowdownReasons: []string{"test reason"},
				HWSlowdown:        true,
				Supported:         true,
			},
			wantJSON: `{"time":"2024-01-01T00:00:00Z","uuid":"GPU-123","reasons_bitmask":8,"hw_slowdown_reasons":["test reason"],"hw_slowdown":true,"hw_thermal_slowdown":false,"hw_slowdown_power_brake":false,"supported":true}`,
			wantYAML: `hw_slowdown: true
hw_slowdown_power_brake: false
hw_slowdown_reasons:
- test reason
hw_thermal_slowdown: false
reasons_bitmask: 8
supported: true
time: "2024-01-01T00:00:00Z"
uuid: GPU-123
`,
		},
		{
			name:        "nil clock events",
			clockEvents: nil,
			wantJSON:    "",
			wantYAML:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON marshaling
			gotJSON, err := tt.clockEvents.JSON()
			if err != nil {
				t.Errorf("ClockEvents.JSON() error = %v", err)
				return
			}
			if string(gotJSON) != tt.wantJSON {
				t.Errorf("ClockEvents.JSON() = %v, want %v", string(gotJSON), tt.wantJSON)
			}

			// Test YAML marshaling
			gotYAML, err := tt.clockEvents.YAML()
			if err != nil {
				t.Errorf("ClockEvents.YAML() error = %v", err)
				return
			}
			if string(gotYAML) != tt.wantYAML {
				t.Errorf("ClockEvents.YAML() = %v, want %v", string(gotYAML), tt.wantYAML)
			}
		})
	}
}

func TestCreateEventFromClockEvents(t *testing.T) {
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		clockEvents ClockEvents
		want        *apiv1.Event
	}{
		{
			name: "no hardware slowdown reasons",
			clockEvents: ClockEvents{
				Time: metav1.Time{Time: testTime},
				UUID: "GPU-123",
			},
			want: nil,
		},
		{
			name: "with hardware slowdown reasons",
			clockEvents: ClockEvents{
				Time:              metav1.Time{Time: testTime},
				UUID:              "GPU-123",
				HWSlowdownReasons: []string{"reason1", "reason2"},
			},
			want: &apiv1.Event{
				Time:    metav1.Time{Time: testTime},
				Name:    "hw_slowdown",
				Type:    apiv1.EventTypeWarning,
				Message: "reason1, reason2",
				ExtraInfo: map[string]string{
					"data_source": "nvml",
					"gpu_uuid":    "GPU-123",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.clockEvents.Event()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Event() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGetClockEventsWithNotSupported adds a test for the not supported case
func TestGetClockEventsWithNotSupported(t *testing.T) {
	testUUID := "GPU-ABCDEF"

	// Create a custom error case - mimicking driver versions < 535
	// which don't support clock events
	t.Run("not supported through string matching", func(t *testing.T) {
		// Override the error string function
		originalErrorString := nvml.ErrorString
		defer func() { nvml.ErrorString = originalErrorString }()

		// Custom return value
		customNotSupportedReturn := nvml.Return(2000)

		nvml.ErrorString = func(ret nvml.Return) string {
			if ret == customNotSupportedReturn {
				return "this operation is not supported in the current driver"
			}
			return originalErrorString(ret)
		}

		// Mock device with custom not supported error
		mockDevice := &testutil.MockDevice{
			Device: &mock.Device{
				GetCurrentClocksEventReasonsFunc: func() (uint64, nvml.Return) {
					return 0, customNotSupportedReturn
				},
			},
		}

		// Call the function
		clockEvents, err := GetClockEvents(testUUID, mockDevice)

		// Should recognize as not supported
		assert.NoError(t, err)
		assert.Equal(t, testUUID, clockEvents.UUID)
		assert.False(t, clockEvents.Supported)
		assert.Equal(t, uint64(0), clockEvents.ReasonsBitmask)
	})
}

// TestClockEventsWithNilPointer tests handling of nil pointers in JSON and YAML serialization
func TestClockEventsWithNilPointer(t *testing.T) {
	// Test handling nil pointers for both JSON and YAML methods
	t.Run("nil receiver for JSON()", func(t *testing.T) {
		var clockEvents *ClockEvents
		result, err := clockEvents.JSON()
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("nil receiver for YAML()", func(t *testing.T) {
		var clockEvents *ClockEvents
		result, err := clockEvents.YAML()
		assert.NoError(t, err)
		assert.Nil(t, result)
	})
}

// TestClockEventsSupportedWithMockedNVML tests the top-level ClockEventsSupported function
// with a mocked NVML library and various device configurations
func TestClockEventsSupportedWithMockedNVML(t *testing.T) {
	// Test with initialization failure
	t.Run("nvml initialization failure", func(t *testing.T) {
		err := os.Setenv(nvml_lib.EnvMockAllSuccess, "true")
		if err != nil {
			t.Fatalf("failed to set mock NVML environment: %v", err)
		}
		defer os.Unsetenv(nvml_lib.EnvMockAllSuccess)

		// Mock NVML with init failure
		mockNVML := &mock.Interface{
			InitFunc: func() nvml.Return {
				return nvml.ERROR_UNKNOWN
			},
		}

		// Replace the mock instance
		originalMockInstance := nvml_lib_mock.AllSuccessInterface
		nvml_lib_mock.AllSuccessInterface = mockNVML
		defer func() { nvml_lib_mock.AllSuccessInterface = originalMockInstance }()

		// Call function
		result, err := ClockEventsSupported()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to initialize NVML")
		assert.False(t, result)
	})

	// Test with device initialization but GetDevices failure
	t.Run("device get failure", func(t *testing.T) {
		err := os.Setenv(nvml_lib.EnvMockAllSuccess, "true")
		if err != nil {
			t.Fatalf("failed to set mock NVML environment: %v", err)
		}
		defer os.Unsetenv(nvml_lib.EnvMockAllSuccess)

		// Mock NVML with device get failure
		mockNVML := &mock.Interface{
			InitFunc: func() nvml.Return {
				return nvml.SUCCESS
			},
			DeviceGetCountFunc: func() (int, nvml.Return) {
				return 0, nvml.ERROR_UNKNOWN
			},
		}

		// Replace the mock instance
		originalMockInstance := nvml_lib_mock.AllSuccessInterface
		nvml_lib_mock.AllSuccessInterface = mockNVML
		defer func() { nvml_lib_mock.AllSuccessInterface = originalMockInstance }()

		// Call function
		result, err := ClockEventsSupported()
		assert.Error(t, err)
		assert.False(t, result)
	})
}
