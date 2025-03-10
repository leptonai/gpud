package config

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestConfigValidate_AutoUpdateExitCode(t *testing.T) {
	tests := []struct {
		name               string
		enableAutoUpdate   bool
		autoUpdateExitCode int
		wantErr            bool
	}{
		{
			name:               "Valid: Auto update enabled with exit code",
			enableAutoUpdate:   true,
			autoUpdateExitCode: 0,
			wantErr:            false,
		},
		{
			name:               "Valid: Auto update disabled with default exit code",
			enableAutoUpdate:   false,
			autoUpdateExitCode: -1,
			wantErr:            false,
		},
		{
			name:               "Invalid: Auto update disabled with non-default exit code",
			enableAutoUpdate:   false,
			autoUpdateExitCode: 0,
			wantErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				RetentionPeriod:    metav1.Duration{Duration: time.Hour},
				CompactPeriod:      metav1.Duration{Duration: time.Hour},
				Address:            "localhost:8080", // Add a valid address to pass other validations
				EnableAutoUpdate:   tt.enableAutoUpdate,
				AutoUpdateExitCode: tt.autoUpdateExitCode,
			}

			err := cfg.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != ErrInvalidAutoUpdateExitCode {
				t.Errorf("Config.Validate() error = %v, want %v", err, ErrInvalidAutoUpdateExitCode)
			}
		})
	}
}
