package nvlink

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		match bool
	}{
		{
			name:  "incident GH100 signature",
			line:  "NVRM: knvlinkDiscoverPostRxDetLinks_GH100: Getting peer0's postRxDetLinkMask failed!",
			match: true,
		},
		{
			name:  "incident link-mask update signature",
			line:  "NVRM: knvlinkUpdatePostRxDetectLinkMask_IMPL: Failed to update Rx Detect Link mask!",
			match: true,
		},
		{
			name:  "other GPU architecture and peer",
			line:  "kernel: NVRM: knvlinkDiscoverPostRxDetLinks_GB100_REV2: Getting peer17 postRxDetLinkMask failed!",
			match: true,
		},
		{
			name: "unrelated NVLink message",
			line: "NVRM: NVLink link training completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventName, message := Match(tt.line)
			if tt.match {
				assert.Equal(t, EventNamePostRxDetectFailure, eventName)
				assert.Equal(t, postRxDetectFailureMessage, message)
			} else {
				assert.Empty(t, eventName)
				assert.Empty(t, message)
			}
		})
	}
}

func TestMatchWithBootID(t *testing.T) {
	line := "NVRM: knvlinkDiscoverPostRxDetLinks_GH100: Getting peer0's postRxDetLinkMask failed!"

	eventName, message := matchWithBootID(line, "boot-1")
	_, nextBootMessage := matchWithBootID(line, "boot-2")

	assert.Equal(t, EventNamePostRxDetectFailure, eventName)
	assert.Equal(t, postRxDetectFailureMessage+" (boot ID: boot-1)", message)
	assert.NotEqual(t, message, nextBootMessage)
}
