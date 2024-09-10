package container

import (
	"errors"
	"testing"
)

func TestIsDockerRunning(t *testing.T) {
	t.Logf("%v", IsDockerRunning())
}

func TestIsErrDockerClientVersionNewerThanDaemon(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Correct error message",
			err:      errors.New("Error response from daemon: client version 1.44 is too new. Maximum supported API version is 1.43"),
			expected: true,
		},
		{
			name:     "Partial match - missing 'is too new'",
			err:      errors.New("Error response from daemon: client version 1.44. Maximum supported API version is 1.43"),
			expected: false,
		},
		{
			name:     "Partial match - missing 'client version'",
			err:      errors.New("Error response from daemon: Docker 1.44 is too new. Maximum supported API version is 1.43"),
			expected: false,
		},
		{
			name:     "Unrelated error message",
			err:      errors.New("Connection refused"),
			expected: false,
		},
		{
			name:     "Nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsErrDockerClientVersionNewerThanDaemon(tt.err)
			if result != tt.expected {
				t.Errorf("IsErrDockerClientVersionNewerThanDaemon() = %v, want %v", result, tt.expected)
			}
		})
	}
}
