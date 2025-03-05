package ecc

import (
	"reflect"
	"testing"

	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func TestToOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    *nvidia_query.Output
		expected *Output
	}{
		{
			name:     "Nil input",
			input:    nil,
			expected: &Output{},
		},
		{
			name: "Empty input",
			input: &nvidia_query.Output{
				SMI:  &nvidia_query.SMIOutput{},
				NVML: &nvidia_query_nvml.Output{},
			},
			expected: &Output{},
		},
		{
			name: "NVML data only",
			input: &nvidia_query.Output{
				NVML: &nvidia_query_nvml.Output{
					DeviceInfos: []*nvidia_query_nvml.DeviceInfo{
						{
							UUID: "GPU-2",
							ECCErrors: nvidia_query_nvml.ECCErrors{
								Volatile: nvidia_query_nvml.AllECCErrorCounts{
									Total: nvidia_query_nvml.ECCErrorCounts{
										Uncorrected: 20,
									},
								},
							},
						},
					},
				},
			},
			expected: &Output{
				ECCModes: []nvidia_query_nvml.ECCMode{
					{},
				},
				ErrorCountsNVML: []nvidia_query_nvml.ECCErrors{
					{
						Volatile: nvidia_query_nvml.AllECCErrorCounts{
							Total: nvidia_query_nvml.ECCErrorCounts{
								Uncorrected: 20,
							},
						},
					},
				},
				VolatileUncorrectedErrorsFromNVML: []string{"[GPU-2] total uncorrected 20 errors"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToOutput(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ToOutput()\n%+v\n\nwant\n%+v", result, tt.expected)
			}

			b, err := result.JSON()
			if err != nil {
				t.Errorf("JSON()\n%+v", err)
			}

			parsed, err := ParseOutputJSON(b)
			if err != nil {
				t.Errorf("ParseOutputJSON()\n%+v", err)
			}

			if !reflect.DeepEqual(parsed, result) {
				t.Errorf("ParseOutputJSON()\n%+v\n\nwant\n%+v", parsed, result)
			}
		})
	}
}

func TestToOutputECCMode(t *testing.T) {
	tests := []struct {
		name     string
		input    *nvidia_query.Output
		expected *Output
	}{
		{
			name:     "Nil input",
			input:    nil,
			expected: &Output{},
		},
		{
			name: "Empty input",
			input: &nvidia_query.Output{
				SMI:  &nvidia_query.SMIOutput{},
				NVML: &nvidia_query_nvml.Output{},
			},
			expected: &Output{},
		},
		{
			name: "With ECC mode",
			input: &nvidia_query.Output{
				NVML: &nvidia_query_nvml.Output{
					DeviceInfos: []*nvidia_query_nvml.DeviceInfo{
						{
							ECCMode: nvidia_query_nvml.ECCMode{
								EnabledCurrent: true,
							},
						},
					},
				},
			},
			expected: &Output{
				ECCModes: []nvidia_query_nvml.ECCMode{
					{
						EnabledCurrent: true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToOutput(tt.input)
			if !reflect.DeepEqual(result.ECCModes, tt.expected.ECCModes) {
				t.Errorf("ToOutput()\n%+v\n\nwant\n%+v", result.ECCModes, tt.expected.ECCModes)
			}

			b, err := result.JSON()
			if err != nil {
				t.Errorf("JSON()\n%+v", err)
			}

			parsed, err := ParseOutputJSON(b)
			if err != nil {
				t.Errorf("ParseOutputJSON()\n%+v", err)
			}

			if !reflect.DeepEqual(parsed.ECCModes, result.ECCModes) {
				t.Errorf("ParseOutputJSON()\n%+v\n\nwant\n%+v", parsed.ECCModes, result.ECCModes)
			}
		})
	}
}
