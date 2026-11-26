package host

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeOSRelease(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestReadOSRelease(t *testing.T) {
	for _, tt := range []struct {
		name, content string
		want          map[string]string
	}{
		{
			name:    "double quotes",
			content: "NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"22.04\"\n",
			want:    map[string]string{"NAME": "Ubuntu", "ID": "ubuntu", "VERSION_ID": "22.04"},
		},
		{
			name:    "single quotes, comments and blank lines",
			content: "# generated\n\nID='ubuntu'\n  VERSION_ID='24.04'  \n",
			want:    map[string]string{"ID": "ubuntu", "VERSION_ID": "24.04"},
		},
		{
			name:    "later field wins",
			content: "ID=debian\nID=ubuntu\n",
			want:    map[string]string{"ID": "ubuntu"},
		},
		{
			name:    "value with equals sign",
			content: "HOME_URL=\"https://example.com/?a=b\"\n",
			want:    map[string]string{"HOME_URL": "https://example.com/?a=b"},
		},
		{
			name:    "empty file",
			content: "",
			want:    map[string]string{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadOSRelease(writeOSRelease(t, tt.content))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadOSRelease_MissingFile(t *testing.T) {
	_, err := ReadOSRelease(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

// A file that cannot be read to the end is an error, even if the fields came
// before the bad line.
func TestReadOSRelease_ReadError(t *testing.T) {
	path := writeOSRelease(t, "ID=ubuntu\nVERSION_ID=22.04\nPADDING="+strings.Repeat("a", bufio.MaxScanTokenSize)+"\n")

	got, err := ReadOSRelease(path)
	require.ErrorIs(t, err, bufio.ErrTooLong)
	assert.Nil(t, got)
}
