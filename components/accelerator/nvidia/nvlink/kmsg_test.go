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
				assert.Equal(t, EventNameDriverWedge, eventName)
				assert.Equal(t, driverWedgeMessage, message)
			} else {
				assert.Empty(t, eventName)
				assert.Empty(t, message)
			}
		})
	}
}
