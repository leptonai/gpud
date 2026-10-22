package firmware

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func dpkgStanza(pkg, status, version string, extra ...string) string {
	lines := []string{"Package: " + pkg, "Status: " + status, "Priority: optional", "Architecture: arm64"}
	if version != "" {
		lines = append(lines, "Version: "+version)
	}
	lines = append(lines, extra...)
	return strings.Join(lines, "\n") + "\n"
}

func dpkgStatus(stanzas ...string) string {
	return strings.Join(stanzas, "\n")
}

func TestReadDOCAHostVersion(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   string
	}{
		{
			name: "doca-ofed installed",
			status: dpkgStatus(
				dpkgStanza("libc6", "install ok installed", "2.39-0ubuntu8.4"),
				dpkgStanza("doca-ofed", "install ok installed", "3.2.1-044413"),
			),
			want: "3.2.1-044413",
		},
		{
			name:   "hold ok installed counts as installed",
			status: dpkgStatus(dpkgStanza("doca-ofed", "hold ok installed", "3.2.1-044413")),
			want:   "3.2.1-044413",
		},
		{
			name: "removed package is skipped for the next installed one",
			status: dpkgStatus(
				dpkgStanza("doca-ofed", "deinstall ok config-files", "3.1.0-091000"),
				dpkgStanza("doca-roce", "install ok installed", "3.2.1-044413"),
			),
			want: "3.2.1-044413",
		},
		{
			name: "half-installed package is skipped",
			status: dpkgStatus(
				dpkgStanza("doca-ofed", "install ok unpacked", "3.2.1-044413"),
			),
		},
		{
			name: "priority order wins over file order",
			status: dpkgStatus(
				dpkgStanza("doca-host", "install ok installed", "3.2.1-044413-25.10-ubuntu2404"),
				dpkgStanza("doca-all", "install ok installed", "3.1.0-091000"),
				dpkgStanza("doca-ofed", "install ok installed", "3.2.1-044413"),
			),
			want: "3.2.1-044413",
		},
		{
			name:   "doca-host alone is only the repository and reports nothing",
			status: dpkgStatus(dpkgStanza("doca-host", "install ok installed", "3.2.1-044413-25.10-ubuntu2404")),
		},
		{
			name: "no DOCA package",
			status: dpkgStatus(
				dpkgStanza("libc6", "install ok installed", "2.39-0ubuntu8.4"),
				dpkgStanza("mlnx-ofed-kernel-utils", "install ok installed", "26.01.OFED.26.01.1.0.0.1"),
			),
		},
		{
			name: "continuation lines never set fields",
			status: dpkgStatus(
				dpkgStanza("doca-ofed", "install ok installed", "3.2.1-044413",
					"Description: DOCA meta package",
					" Version: 9.9.9",
					"\tPackage: doca-all",
					" ."),
			),
			want: "3.2.1-044413",
		},
		{
			name:   "installed package without a version is skipped",
			status: dpkgStatus(dpkgStanza("doca-ofed", "install ok installed", "")),
		},
		{
			name:   "last stanza without a trailing blank line",
			status: "Package: doca-all\nStatus: install ok installed\nVersion: 3.2.1-044413",
			want:   "3.2.1-044413",
		},
		{
			name:   "empty file",
			status: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readDOCAHostVersion(strings.NewReader(tt.status))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadDOCAHostVersion_LongLine(t *testing.T) {
	longDepends := "Depends: " + strings.Repeat("libfoo (>= 1.0), ", 8000)
	require.Greater(t, len(longDepends), 64*1024)

	got, err := readDOCAHostVersion(strings.NewReader(dpkgStatus(
		dpkgStanza("doca-all", "install ok installed", "3.2.1-044413", longDepends),
	)))
	require.NoError(t, err)
	assert.Equal(t, "3.2.1-044413", got)
}

func TestCollectDOCAHost(t *testing.T) {
	hostFS := fstest.MapFS{dpkgStatusPath: {Data: []byte(dpkgStatus(
		dpkgStanza("doca-ofed", "install ok installed", "3.2.1-044413"),
	))}}
	assert.Equal(t, []instance{{component: apiv1.FirmwareComponentDOCAHost, version: "3.2.1-044413"}}, collectDOCAHost(hostFS))

	// RPM hosts have no dpkg database.
	assert.Nil(t, collectDOCAHost(fstest.MapFS{}))

	noDOCA := fstest.MapFS{dpkgStatusPath: {Data: []byte(dpkgStanza("libc6", "install ok installed", "2.39"))}}
	assert.Nil(t, collectDOCAHost(noDOCA))
}
