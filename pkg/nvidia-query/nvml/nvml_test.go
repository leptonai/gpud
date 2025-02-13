package nvml

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	mocknvml "github.com/leptonai/gpud/e2e/mock/nvml"
)

func TestParseDriverVersion(t *testing.T) {
	testCases := []struct {
		version    string
		wantMajor  int
		wantMinor  int
		wantPatch  int
		wantErrNil bool
	}{
		{
			version:    "525.85.12",
			wantMajor:  525,
			wantMinor:  85,
			wantPatch:  12,
			wantErrNil: true,
		},
		{
			version:    "535.161.08",
			wantMajor:  535,
			wantMinor:  161,
			wantPatch:  8,
			wantErrNil: true,
		},
		{
			version:    "550.120",
			wantMajor:  550,
			wantMinor:  120,
			wantPatch:  0,
			wantErrNil: true,
		},
		{
			// Empty string
			version:    "",
			wantErrNil: false,
		},
		{
			// Single number
			version:    "525",
			wantErrNil: false,
		},
		{
			// Too many parts
			version:    "525.85.12.1",
			wantErrNil: false,
		},
		{
			// Non-numeric major
			version:    "abc.85.12",
			wantErrNil: false,
		},
		{
			// Non-numeric minor
			version:    "525.abc.12",
			wantErrNil: false,
		},
		{
			// Non-numeric patch
			version:    "525.85.abc",
			wantErrNil: false,
		},
		{
			// Invalid separators
			version:    "525-85-12",
			wantErrNil: false,
		},
		{
			// Leading zeros in patch
			version:    "525.85.08",
			wantMajor:  525,
			wantMinor:  85,
			wantPatch:  8,
			wantErrNil: true,
		},
		{
			// Leading zeros in minor
			version:    "525.085.12",
			wantMajor:  525,
			wantMinor:  85,
			wantPatch:  12,
			wantErrNil: true,
		},
		{
			// Invalid version with spaces
			version:    "525. 85.12",
			wantErrNil: false,
		},
		{
			// Invalid version with trailing dot
			version:    "525.85.",
			wantErrNil: false,
		},
		{
			// Invalid version with leading dot
			version:    ".525.85",
			wantErrNil: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.version, func(t *testing.T) {
			major, minor, patch, err := ParseDriverVersion(tc.version)

			if (err == nil) != tc.wantErrNil {
				t.Errorf("ParseDriverVersion(%q) error = %v, wantErrNil %v", tc.version, err, tc.wantErrNil)
				return
			}

			if err == nil {
				if major != tc.wantMajor {
					t.Errorf("ParseDriverVersion(%q) major = %d, want %d", tc.version, major, tc.wantMajor)
				}
				if minor != tc.wantMinor {
					t.Errorf("ParseDriverVersion(%q) minor = %d, want %d", tc.version, minor, tc.wantMinor)
				}
				if patch != tc.wantPatch {
					t.Errorf("ParseDriverVersion(%q) patch = %d, want %d", tc.version, patch, tc.wantPatch)
				}
			}
		})
	}
}

func TestNewNVML(t *testing.T) {
	nvmlLib := NewNVML()
	assert.NotEqual(t, mocknvml.MockInstance, nvmlLib)

	err := os.Setenv(mocknvml.EnvNVMLMock, "true")
	assert.NoError(t, err)
	defer os.Unsetenv(mocknvml.EnvNVMLMock)
	nvmlLib = NewNVML()
	assert.Equal(t, mocknvml.MockInstance, nvmlLib)
}
