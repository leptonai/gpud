package nvml

import "testing"

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
			version:    "invalid.version",
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
