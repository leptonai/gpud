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
		{name: "linode decimal id with newline", raw: "108254612\n", want: "108254612"},
		{name: "linode decimal id with whitespace", raw: " \t108254616\r\n", want: "108254616"},
		{name: "opaque id preserves case", raw: " \tVendor-Node-A1\n", want: "Vendor-Node-A1"},
		{name: "firebird bm ulid", raw: "bm.01M3KNHF1NFX3VX8Z1EWNQ2H3X\n", want: "bm.01M3KNHF1NFX3VX8Z1EWNQ2H3X"},
		{name: "firebird vm ulid normalizes case", raw: "VM.01m3knhf1nfx3vx8z1ewnq2h3x", want: "vm.01M3KNHF1NFX3VX8Z1EWNQ2H3X"},
		{name: "cloud-init placeholder with whitespace", raw: " \tiid-datasource-none\n", wantErr: true},
		{name: "datasource placeholder with whitespace", raw: " \tiid-datasource\r\n", wantErr: true},
		{name: "nocloud placeholder with whitespace", raw: " \tnocloud\r\n", wantErr: true},
		{name: "config drive placeholder with whitespace", raw: " \tiid-dsconfigdrive\r\n", wantErr: true},
		{name: "ovf placeholder with whitespace", raw: " \tiid-dsovf\r\n", wantErr: true},
		{name: "opennebula placeholder with whitespace", raw: " \tiid-dsopennebula\r\n", wantErr: true},
		{name: "empty", raw: "  \n", wantErr: true},
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

	firebird := write("firebird-instance-id", "bm.01M3KNHF1NFX3VX8Z1EWNQ2H3X\n")
	assert.Equal(t, "bm.01M3KNHF1NFX3VX8Z1EWNQ2H3X", readInstanceIDFallback([]string{firebird, first}),
		"firebird-instance-id takes precedence over cloud-init instance-id when both exist")

	linode := write("linode-instance-id", "108254612\n")
	assert.Equal(t, "108254612", readInstanceIDFallback([]string{missing, placeholder, linode, first}))

	for _, id := range []string{"iid-datasource-none", "iid-datasource", "nocloud", "iid-dsconfigdrive", "iid-dsovf", "iid-dsopennebula"} {
		t.Run(id, func(t *testing.T) {
			placeholder := write(id, " \t"+id+"\r\n")
			assert.Empty(t, readInstanceIDFallback([]string{placeholder}))
			assert.Equal(t, "108254612", readInstanceIDFallback([]string{placeholder, linode, first}),
				"a cloud-init placeholder must not hide a later valid instance ID")
		})
	}
}
