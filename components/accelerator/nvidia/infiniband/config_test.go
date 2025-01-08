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
				"at_least_ports": 4,
				"at_least_rate":  200,
			},
			wantErr: false,
			want: Config{
				ExpectedPortStates: ExpectedPortStates{
					AtLeastPorts: 4,
					AtLeastRate:  200,
				},
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
			got, err := ParseConfig(tt.input, nil, nil)
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
