package tailscale

import (
	"reflect"
	"testing"
)

func TestVersion(t *testing.T) {
	version, err := CheckVersion()
	if err != nil {
		t.Skip(err) // ci may not have tailscale installed
	}
	t.Logf("version: %+v", version)
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    *VersionInfo
		wantErr bool
	}{
		{
			name:  "valid input",
			input: []byte(`{"majorMinorPatch":"1.61.0","short":"1.61.0-ERR-BuildInfo","long":"1.61.0-ERR-BuildInfo","unstableBranch":true,"cap":88}`),
			want: &VersionInfo{
				MajorMinorPatch: "1.61.0",
				Short:           "1.61.0-ERR-BuildInfo",
				Long:            "1.61.0-ERR-BuildInfo",
				UnstableBranch:  true,
				Cap:             88,
			},
			wantErr: false,
		},
		{
			name:    "invalid json",
			input:   []byte(`{"majorMinorPatch":"1.61.0",`),
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseVersion() got = %v, want %v", got, tt.want)
			}
		})
	}
}
