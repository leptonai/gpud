package nvml

import "testing"

func TestRemappedRows_QualifiesForRMA(t *testing.T) {
	tests := []struct {
		name                           string
		remappingFailed                bool
		remappedDueToUncorrectableErrs int
		want                           bool
	}{
		{
			name:                           "qualifies when remapping failed with <8 uncorrectable errors",
			remappingFailed:                true,
			remappedDueToUncorrectableErrs: 5,
			want:                           true,
		},
		{
			name:                           "qualifies when remapping failed with >=8 uncorrectable errors",
			remappingFailed:                true,
			remappedDueToUncorrectableErrs: 8,
			want:                           true,
		},
		{
			name:                           "does not qualify when remapping hasn't failed",
			remappingFailed:                false,
			remappedDueToUncorrectableErrs: 8,
			want:                           false,
		},
		{
			name:                           "does not qualify when remapping hasn't failed with <8 errors",
			remappingFailed:                false,
			remappedDueToUncorrectableErrs: 5,
			want:                           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := RemappedRows{
				RemappingFailed:                  tt.remappingFailed,
				RemappedDueToUncorrectableErrors: tt.remappedDueToUncorrectableErrs,
			}
			if got := r.QualifiesForRMA(); got != tt.want {
				t.Errorf("RemappedRows.QualifiesForRMA() = %v, want %v", got, tt.want)
			}
		})
	}
}
