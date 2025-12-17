package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseInfinibandExcludeDevices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
		{
			name:  "commas and spaces only",
			input: " , , ",
			want:  nil,
		},
		{
			name:  "single device",
			input: "mlx5_0",
			want:  []string{"mlx5_0"},
		},
		{
			name:  "multiple devices with spaces and empties",
			input: " mlx5_0, ,mlx5_1 ,",
			want:  []string{"mlx5_0", "mlx5_1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseInfinibandExcludeDevices(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
