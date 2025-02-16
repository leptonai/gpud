package query

import (
	"reflect"
	"testing"
)

func TestTemperature(t *testing.T) {
	t.Parallel()

	tm := &SMIGPUTemperature{
		Current: "38 C",
		Limit:   "47 C",
	}

	currentTempC, err := tm.GetCurrentCelsius()
	if err != nil {
		t.Fatalf("error getting current temperature: %v", err)
	}
	if currentTempC != 38.0 {
		t.Fatalf("expected current temperature of 38.0, got %f", currentTempC)
	}

	currentLimitTempC, err := tm.GetLimitCelsius()
	if err != nil {
		t.Fatalf("error getting current limit temperature: %v", err)
	}
	if currentLimitTempC != 47.0 {
		t.Fatalf("expected current limit temperature of 47.0, got %f", currentLimitTempC)
	}
}

func TestGPUPowerReadings(t *testing.T) {
	t.Parallel()

	g := &SMIGPUPowerReadings{
		PowerDraw:         "71.97 W",
		CurrentPowerLimit: "700.00 W",
	}
	powerDrawW, err := g.GetPowerDrawW()
	if err != nil {
		t.Fatalf("error getting power draw: %v", err)
	}
	if powerDrawW != 71.97 {
		t.Fatalf("expected power draw of 71.97, got %f", powerDrawW)
	}
	currentPowerLimitW, err := g.GetCurrentPowerLimitW()
	if err != nil {
		t.Fatalf("error getting current power limit: %v", err)
	}
	if currentPowerLimitW != 700 {
		t.Fatalf("expected current power limit of 700, got %f", currentPowerLimitW)
	}
}

func TestGPU_HasErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		gpu    NvidiaSMIGPU
		errors []string
	}{
		{
			name:   "No errors",
			gpu:    NvidiaSMIGPU{Temperature: &SMIGPUTemperature{}, FanSpeed: "50%"},
			errors: nil,
		},
		{
			name:   "Error in Temperature.Current",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{Current: "Unknown Error"}, FanSpeed: "50%"},
			errors: []string{"test: Temperature.Current Unknown Error"},
		},
		{
			name:   "Error in Temperature.Limit",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{Limit: "Unknown Error"}, FanSpeed: "50%"},
			errors: []string{"test: Temperature.Limit Unknown Error"},
		},
		{
			name:   "Error in Temperature.ShutdownLimit",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{ShutdownLimit: "Unknown Error"}, FanSpeed: "50%"},
			errors: []string{"test: Temperature.ShutdownLimit Unknown Error"},
		},
		{
			name:   "Error in Temperature.SlowdownLimit",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{SlowdownLimit: "Unknown Error"}, FanSpeed: "50%"},
			errors: []string{"test: Temperature.SlowdownLimit Unknown Error"},
		},
		{
			name:   "Error in Temperature.MaxOperatingLimit",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{MaxOperatingLimit: "Unknown Error"}, FanSpeed: "50%"},
			errors: []string{"test: Temperature.MaxOperatingLimit Unknown Error"},
		},
		{
			name:   "Error in Temperature.Target",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{Target: "Unknown Error"}, FanSpeed: "50%"},
			errors: []string{"test: Temperature.Target Unknown Error"},
		},
		{
			name:   "Error in Temperature.MemoryCurrent",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{MemoryCurrent: "Unknown Error"}, FanSpeed: "50%"},
			errors: []string{"test: Temperature.MemoryCurrent Unknown Error"},
		},
		{
			name:   "Error in Temperature.MemoryMaxOperatingLimit",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{MemoryMaxOperatingLimit: "Unknown Error"}, FanSpeed: "50%"},
			errors: []string{"test: Temperature.MemoryMaxOperatingLimit Unknown Error"},
		},
		{
			name:   "Error in FanSpeed",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: &SMIGPUTemperature{}, FanSpeed: "Unknown Error"},
			errors: []string{"test: FanSpeed Unknown Error"},
		},
		{
			name:   "Nil Temperature",
			gpu:    NvidiaSMIGPU{ID: "test", Temperature: nil, FanSpeed: "50%"},
			errors: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.gpu.FindErrs(); !reflect.DeepEqual(got, tt.errors) {
				t.Errorf("GPU.HasErr() = %v, want %v", got, tt.errors)
			}
		})
	}
}
