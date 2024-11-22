package query

import "strings"

// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
// "error getting device handle for index '6': Unknown Error"
func IsErrDeviceHandleUnknownError(err error) bool {
	if err == nil {
		return false
	}

	e := strings.ToLower(err.Error())
	return strings.Contains(e, "error getting device handle") &&
		strings.Contains(e, "unknown error")
}
