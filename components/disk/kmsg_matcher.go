package disk

import (
	"regexp"
)

const (
	// e.g.,
	// md/raid0md0: Disk failure on nvme0n1p1 detected, failing array.
	eventRAIDArrayFailure   = "raid_array_failure"
	regexRAIDArrayFailure   = `md/raid.*: Disk failure on .* detected, failing array`
	messageRAIDArrayFailure = `RAID array has failed due to disk failure`

	// e.g.,
	// EXT4-fs (md0): Remounting filesystem read-only
	// XFS (sda1): Remounting filesystem read-only
	// BTRFS: Remounting filesystem read-only
	eventFilesystemReadOnly   = "filesystem_read_only"
	regexFilesystemReadOnly   = `.*Remounting filesystem read-only`
	messageFilesystemReadOnly = `filesystem remounted as read-only due to errors`

	// e.g.,
	// block nvme0n1: no available path - failing I/O
	eventNVMePathFailure   = "nvme_path_failure"
	regexNVMePathFailure   = `block nvme.*: no available path - failing I/O`
	messageNVMePathFailure = `NVMe device has no available path, I/O failing`

	// e.g.,
	// nvme nvme4: I/O tag 22 (2016) opcode 0x2 (Admin Cmd) QID 0 timeout, reset controller
	eventNVMeTimeout   = "nvme_controller_timeout"
	regexNVMeTimeout   = `nvme nvme[0-9]+: I/O .* timeout, reset controller`
	messageNVMeTimeout = `NVME controller I/O timeout detected, attempting reset`

	// e.g.,
	// nvme nvme4: Disabling device after reset failure: -19
	eventNVMeDeviceDisabled   = "nvme_device_disabled"
	regexNVMeDeviceDisabled   = `nvme nvme[0-9]+: Disabling device after reset failure`
	messageNVMeDeviceDisabled = `NVME device disabled after reset failure`

	// e.g.,
	// kworker/u203:1: attempt to access beyond end of device
	eventBeyondEndOfDevice   = "beyond_end_of_device"
	regexBeyondEndOfDevice   = `attempt to access beyond end of device`
	messageBeyondEndOfDevice = `I/O attempt beyond device boundaries detected`

	// e.g.,
	// Buffer I/O error on dev dm-0, logical block 1308098575, lost async page write
	eventBufferIOError   = "buffer_io_error"
	regexBufferIOError   = `Buffer I/O error on dev [^ ]+, logical block [0-9]+`
	messageBufferIOError = `Buffer I/O error detected on device`

	// e.g.,
	// EXT4-fs (dm-0): I/O error while writing superblock
	eventSuperblockWriteError   = "superblock_write_error"
	regexSuperblockWriteError   = `I/O error while writing superblock`
	messageSuperblockWriteError = `I/O error while writing superblock`
)

var (
	compiledRAIDArrayFailure     = regexp.MustCompile(regexRAIDArrayFailure)
	compiledFilesystemReadOnly   = regexp.MustCompile(regexFilesystemReadOnly)
	compiledNVMePathFailure      = regexp.MustCompile(regexNVMePathFailure)
	compiledNVMeTimeout          = regexp.MustCompile(regexNVMeTimeout)
	compiledNVMeDeviceDisabled   = regexp.MustCompile(regexNVMeDeviceDisabled)
	compiledBeyondEndOfDevice    = regexp.MustCompile(regexBeyondEndOfDevice)
	compiledBufferIOError        = regexp.MustCompile(regexBufferIOError)
	compiledSuperblockWriteError = regexp.MustCompile(regexSuperblockWriteError)
)

// Returns true if the line indicates a RAID array failure.
func HasRAIDArrayFailure(line string) bool {
	if match := compiledRAIDArrayFailure.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// Returns true if the line indicates filesystem was remounted read-only.
func HasFilesystemReadOnly(line string) bool {
	if match := compiledFilesystemReadOnly.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// Returns true if the line indicates NVMe path failure.
func HasNVMePathFailure(line string) bool {
	if match := compiledNVMePathFailure.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// Returns true if the line indicates NVMe controller timeout.
func HasNVMeTimeout(line string) bool {
	if match := compiledNVMeTimeout.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// Returns true if the line indicates NVMe device was disabled.
func HasNVMeDeviceDisabled(line string) bool {
	if match := compiledNVMeDeviceDisabled.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// Returns true if the line indicates attempt to access beyond end of device.
func HasBeyondEndOfDevice(line string) bool {
	if match := compiledBeyondEndOfDevice.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// Returns true if the line indicates buffer I/O error.
func HasBufferIOError(line string) bool {
	if match := compiledBufferIOError.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// Returns true if the line indicates superblock write error.
func HasSuperblockWriteError(line string) bool {
	if match := compiledSuperblockWriteError.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func Match(line string) (eventName string, message string) {
	for _, m := range getMatches() {
		if m.check(line) {
			return m.eventName, m.message
		}
	}
	return "", ""
}

type match struct {
	check     func(string) bool
	eventName string
	regex     string
	message   string
}

func getMatches() []match {
	return []match{
		{check: HasRAIDArrayFailure, eventName: eventRAIDArrayFailure, regex: regexRAIDArrayFailure, message: messageRAIDArrayFailure},
		{check: HasFilesystemReadOnly, eventName: eventFilesystemReadOnly, regex: regexFilesystemReadOnly, message: messageFilesystemReadOnly},
		{check: HasNVMePathFailure, eventName: eventNVMePathFailure, regex: regexNVMePathFailure, message: messageNVMePathFailure},
		{check: HasNVMeTimeout, eventName: eventNVMeTimeout, regex: regexNVMeTimeout, message: messageNVMeTimeout},
		{check: HasNVMeDeviceDisabled, eventName: eventNVMeDeviceDisabled, regex: regexNVMeDeviceDisabled, message: messageNVMeDeviceDisabled},
		{check: HasBeyondEndOfDevice, eventName: eventBeyondEndOfDevice, regex: regexBeyondEndOfDevice, message: messageBeyondEndOfDevice},
		{check: HasBufferIOError, eventName: eventBufferIOError, regex: regexBufferIOError, message: messageBufferIOError},
		{check: HasSuperblockWriteError, eventName: eventSuperblockWriteError, regex: regexSuperblockWriteError, message: messageSuperblockWriteError},
	}
}
