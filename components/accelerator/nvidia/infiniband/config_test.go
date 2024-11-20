package infiniband

import (
	"reflect"
	"testing"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]interface{}
		wantErr bool
		want    Config
	}{
		{
			name: "valid config",
			input: map[string]interface{}{
				"expected_port_count": 4,
				"expected_rate":       200,
			},
			wantErr: false,
			want: Config{
				ExpectedPortCount: 4,
				ExpectedRate:      200,
			},
		},
		{
			name:    "empty config",
			input:   map[string]interface{}{},
			wantErr: false,
			want:    Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig(tt.input, nil)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseConfig() error = nil, wantErr = true")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseConfig() error = %v, wantErr = false", err)
				return
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("ParseConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name       string
		config     Config
		wantError  bool
		wantConfig Config
	}{
		{
			name:   "zero expected rate should set default",
			config: Config{ExpectedRate: 0},
			wantConfig: Config{
				ExpectedRate: DefaultExpectedRate,
			},
			wantError: false,
		},
		{
			name:   "non-zero expected rate should remain unchanged",
			config: Config{ExpectedRate: 200},
			wantConfig: Config{
				ExpectedRate: 200,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				if err == nil {
					t.Errorf("Validate() error = nil, wantErr = true")
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() error = %v, wantErr = false", err)
				return
			}
			if tt.config.ExpectedRate != tt.wantConfig.ExpectedRate {
				t.Errorf("ExpectedRate = %v, want %v", tt.config.ExpectedRate, tt.wantConfig.ExpectedRate)
			}
		})
	}
}
