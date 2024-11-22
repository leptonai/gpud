package query

import "strings"

// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
// "error getting device handle for index '6': Unknown Error"
//
// or
// "Unable to determine the device handle for GPU0000:CB:00.0: Unknown Error"
func IsErrDeviceHandleUnknownError(err error) bool {
	if err == nil {
		return false
	}

	e := strings.ToLower(err.Error())
	if !strings.Contains(e, "unknown error") {
		return false
	}
	return strings.Contains(e, "error getting device handle") ||
		strings.Contains(e, "unable to determine the device handle")
}
