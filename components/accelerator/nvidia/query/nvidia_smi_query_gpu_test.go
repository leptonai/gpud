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

func TestFBMemoryUsage(t *testing.T) {
	t.Parallel()

	f := &SMIFBMemoryUsage{
		Total:    "81559 MiB",
		Reserved: "551 MiB",
		Used:     "0 MiB",
		Free:     "81007 MiB",
	}

	totalBytes, err := f.GetTotalBytes()
	if err != nil {
		t.Fatalf("error getting total bytes: %v", err)
	}
	if totalBytes != 85520809984 {
		t.Fatalf("expected total bytes of 85520809984, got %d", totalBytes)
	}

	reservedBytes, err := f.GetReservedBytes()
	if err != nil {
		t.Fatalf("error getting reserved bytes: %v", err)
	}
	if reservedBytes != 577765376 {
		t.Fatalf("expected reserved bytes of 577765376, got %d", reservedBytes)
	}

	usedBytes, err := f.GetUsedBytes()
	if err != nil {
		t.Fatalf("error getting used bytes: %v", err)
	}
	if usedBytes != 0 {
		t.Fatalf("expected used bytes of 0, got %d", usedBytes)
	}

	freeBytes, err := f.GetFreeBytes()
	if err != nil {
		t.Fatalf("error getting free bytes: %v", err)
	}
	if freeBytes != 84941996032 {
		t.Fatalf("expected free bytes of 84941996032, got %d", freeBytes)
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

func TestParsedSMIRemappedRows_QualifiesForRMA(t *testing.T) {
	tests := []struct {
		name    string
		rows    ParsedSMIRemappedRows
		want    bool
		wantErr bool
	}{
		{
			name: "qualifies for RMA when remapping failed with <8 uncorrectable errors",
			rows: ParsedSMIRemappedRows{
				RemappedDueToUncorrectableErrors: "5",
				RemappingFailed:                  "Yes",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "qualifies for RMA when remapping failed with >=8 uncorrectable errors",
			rows: ParsedSMIRemappedRows{
				RemappedDueToUncorrectableErrors: "10",
				RemappingFailed:                  "Yes",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "does not qualify for RMA when remapping succeeded",
			rows: ParsedSMIRemappedRows{
				RemappedDueToUncorrectableErrors: "5",
				RemappingFailed:                  "No",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "returns error for invalid remapping failed value",
			rows: ParsedSMIRemappedRows{
				RemappedDueToUncorrectableErrors: "5",
				RemappingFailed:                  "Invalid",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "returns error for invalid uncorrectable errors value",
			rows: ParsedSMIRemappedRows{
				RemappedDueToUncorrectableErrors: "invalid",
				RemappingFailed:                  "Yes",
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.rows.QualifiesForRMA()
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsedSMIRemappedRows.QualifiesForRMA() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParsedSMIRemappedRows.QualifiesForRMA() = %v, want %v", got, tt.want)
			}
		})
	}
}
