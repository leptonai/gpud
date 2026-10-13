package machineinfo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFallbackInstanceID(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "config drive metal3 uuid", raw: "2b1434ed-d9fc-489f-9fd4-7859292d309f\n", want: "2b1434ed-d9fc-489f-9fd4-7859292d309f"},
		{name: "uppercase uuid", raw: "EBFD7E9B-FA1B-4D27-9603-58FFD1A2571A", want: "ebfd7e9b-fa1b-4d27-9603-58ffd1a2571a"},
		{name: "32 hex", raw: "76cc44587972452dbcd3787596b86407", want: "76cc44587972452dbcd3787596b86407"},
		{name: "cloud-init placeholder", raw: "iid-datasource-none", wantErr: true},
		{name: "empty", raw: "  \n", wantErr: true},
		{name: "misplaced dashes", raw: "2b1434edd-9fc-489f-9fd4-7859292d309f", wantErr: true},
		{name: "non-hex uuid", raw: "zb1434ed-d9fc-489f-9fd4-7859292d309f", wantErr: true},
		{name: "ec2 style id", raw: "i-0abc1234567890def", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFallbackInstanceID(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadInstanceIDFallback(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
		return p
	}
	missing := filepath.Join(dir, "missing")
	placeholder := write("placeholder", "iid-datasource-none")
	first := write("first", "2b1434ed-d9fc-489f-9fd4-7859292d309f")
	second := write("second", "b20924c0-ab35-4b47-940b-23f7fede4ac0")

	assert.Equal(t, "2b1434ed-d9fc-489f-9fd4-7859292d309f", readInstanceIDFallback([]string{missing, placeholder, first, second}),
		"missing and invalid files are skipped; the first valid file wins")
	assert.Empty(t, readInstanceIDFallback([]string{missing, placeholder}))
	assert.Empty(t, readInstanceIDFallback(nil))
}
