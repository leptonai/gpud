package nvlink

import "regexp"

const (
	// EventNameDriverWedge is emitted when GPUd detects a wedged NVIDIA NVLink driver.
	EventNameDriverWedge = "nvlink_driver_wedge"

	// RegexDriverWedgeKMessage matches the NVIDIA driver failure seen when
	// NVLink link discovery wedges while reading the post-RX detection mask.
	RegexDriverWedgeKMessage = `NVRM: knvlinkDiscoverPostRxDetLinks_[A-Za-z0-9_]+: Getting peer[0-9]+(?:'s)? postRxDetLinkMask failed!`

	driverWedgeMessage = "NVIDIA driver is stuck retrying NVLink link discovery (postRxDetLinkMask failure)"
)

var compiledRegexDriverWedgeKMessage = regexp.MustCompile(RegexDriverWedgeKMessage)

// HasDriverWedge returns true if the line indicates a wedged NVIDIA NVLink driver.
func HasDriverWedge(line string) bool {
	return compiledRegexDriverWedgeKMessage.MatchString(line)
}

// Match returns the normalized event name and message for a matching kmsg line.
func Match(line string) (eventName string, message string) {
	if HasDriverWedge(line) {
		return EventNameDriverWedge, driverWedgeMessage
	}
	return "", ""
}

func matchWithBootID(line string, bootID string) (eventName string, message string) {
	eventName, message = Match(line)
	if eventName != "" {
		message += " (boot ID: " + bootID + ")"
	}
	return eventName, message
}
