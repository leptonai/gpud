package nvlink

import "regexp"

const (
	// EventNamePostRxDetectFailure is emitted when the NVIDIA driver cannot
	// update an NVLink post-RX-detection link mask.
	EventNamePostRxDetectFailure = "nvlink_post_rx_detect_failure"

	// NVIDIA's published GH100 discovery path calls
	// knvlinkUpdatePostRxDetectLinkMask and reports the peer-mask failure when
	// that update fails. Some driver builds also emit the paired update failure.
	// The source does not classify this error as a driver hang.
	// ref. https://github.com/NVIDIA/open-gpu-kernel-modules/blob/452cec62d827034798072827d3866d1881662b77/src/nvidia/src/kernel/gpu/nvlink/arch/hopper/kernel_nvlink_gh100.c#L173-L249
	RegexPostRxDetectFailureKMessage = `NVRM: (?:knvlinkUpdatePostRxDetectLinkMask_[A-Za-z0-9_]+: Failed to update Rx Detect Link mask!|knvlinkDiscoverPostRxDetLinks_[A-Za-z0-9_]+: Getting peer[0-9]+(?:'s)? postRxDetLinkMask failed!)`

	postRxDetectFailureMessage = "NVIDIA driver failed to update an NVLink post-RX-detection link mask"
)

var compiledRegexPostRxDetectFailureKMessage = regexp.MustCompile(RegexPostRxDetectFailureKMessage)

// HasPostRxDetectFailure returns true if the line reports an NVLink post-RX-detection failure.
func HasPostRxDetectFailure(line string) bool {
	return compiledRegexPostRxDetectFailureKMessage.MatchString(line)
}

// Match returns the normalized event name and message for a matching kmsg line.
func Match(line string) (eventName string, message string) {
	if HasPostRxDetectFailure(line) {
		return EventNamePostRxDetectFailure, postRxDetectFailureMessage
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
