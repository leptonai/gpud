package dmesg

import (
	"testing"
)

func Test_checkDmesgVersionOutputForSinceFlag(t *testing.T) {
	tests := []struct {
		name string
		ver  string
		want bool
	}{
		{
			name: "version 2.37.2",
			ver:  "dmesg from util-linux 2.37.2",
			want: true,
		},
		{
			name: "version 2.26",
			ver:  "dmesg from util-linux 2.26",
			want: false,
		},
		{
			name: "version 2.49",
			ver:  "dmesg from util-linux 2.49",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkDmesgVersionOutputForSinceFlag(tt.ver)
			if got != tt.want {
				t.Errorf("checkDmesgVersionOutputForSinceFlag() = %v, want %v", got, tt.want)
			}
		})
	}
}
