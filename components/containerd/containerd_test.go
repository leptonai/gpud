package containerd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseContainerdVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "with v prefix",
			input: "containerd github.com/containerd/containerd v1.7.20 abc",
			want:  "v1.7.20",
		},
		{
			name:  "without v prefix",
			input: "containerd containerd.io 1.7.20 abc",
			want:  "1.7.20",
		},
		{
			name:    "invalid output",
			input:   "containerd version",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContainerdVersion(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
