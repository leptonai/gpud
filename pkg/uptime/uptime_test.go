package uptime

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCurrentProcessStartTimeInUnixTime(t *testing.T) {
	startTime, err := GetCurrentProcessStartTimeInUnixTime()

	switch runtime.GOOS {
	case "darwin", "windows":
		// On Darwin/Windows, returns 0 with no error
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), startTime)
	case "linux":
		// On Linux, should return a valid timestamp
		assert.NoError(t, err)
		// Start time should be a reasonable Unix timestamp (after year 2000)
		// Year 2000 in Unix time is approximately 946684800
		if err == nil {
			assert.True(t, startTime > 946684800 || startTime == 0,
				"start time should be after year 2000 or 0 on error")
		}
	}
}
