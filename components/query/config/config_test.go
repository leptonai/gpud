package config

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetDefaultsIfNotSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    Config
		expected Config
	}{
		{
			name:  "All defaults",
			input: Config{},
			expected: Config{
				Interval:  metav1.Duration{Duration: DefaultPollInterval},
				QueueSize: DefaultQueueSize,
			},
		},
		{
			name: "Custom interval",
			input: Config{
				Interval: metav1.Duration{Duration: 5 * time.Minute},
			},
			expected: Config{
				Interval:  metav1.Duration{Duration: 5 * time.Minute},
				QueueSize: DefaultQueueSize,
			},
		},
		{
			name: "Custom queue size",
			input: Config{
				QueueSize: 100,
			},
			expected: Config{
				Interval:  metav1.Duration{Duration: DefaultPollInterval},
				QueueSize: 100,
			},
		},
		{
			name: "State enabled without retention",
			input: Config{
				State: &State{},
			},
			expected: Config{
				Interval:  metav1.Duration{Duration: DefaultPollInterval},
				QueueSize: DefaultQueueSize,
				State: &State{
					Retention: metav1.Duration{Duration: DefaultStateRetention},
				},
			},
		},
		{
			name: "State enabled with custom retention",
			input: Config{
				State: &State{
					Retention: metav1.Duration{Duration: 2 * time.Hour},
				},
			},
			expected: Config{
				Interval:  metav1.Duration{Duration: DefaultPollInterval},
				QueueSize: DefaultQueueSize,
				State: &State{
					Retention: metav1.Duration{Duration: 2 * time.Hour},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.SetDefaultsIfNotSet()

			if tt.input.Interval.Duration != tt.expected.Interval.Duration {
				t.Errorf("Interval mismatch: got %v, want %v", tt.input.Interval.Duration, tt.expected.Interval.Duration)
			}

			if tt.input.QueueSize != tt.expected.QueueSize {
				t.Errorf("QueueSize mismatch: got %d, want %d", tt.input.QueueSize, tt.expected.QueueSize)
			}

			if (tt.input.State == nil) != (tt.expected.State == nil) {
				t.Errorf("State mismatch: got %v, want %v", tt.input.State, tt.expected.State)
			}

			if tt.input.State != nil && tt.expected.State != nil {
				if tt.input.State.Retention.Duration != tt.expected.State.Retention.Duration {
					t.Errorf("State.Retention mismatch: got %v, want %v", tt.input.State.Retention.Duration, tt.expected.State.Retention.Duration)
				}
			}
		})
	}
}
