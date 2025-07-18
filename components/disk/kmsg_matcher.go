package disk

import (
	"regexp"
)

const (
	// e.g.,
	// [Sun Dec  8 09:23:39 2024] systemd-journald[889]: Failed to open system journal: No space left on device
	eventNoSpaceLeft   = "no_space_left"
	regexNoSpaceLeft   = `No space left`
	messageNoSpaceLeft = `no disk space left`

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
)

var (
	compiledNoSpaceLeft        = regexp.MustCompile(regexNoSpaceLeft)
	compiledRAIDArrayFailure   = regexp.MustCompile(regexRAIDArrayFailure)
	compiledFilesystemReadOnly = regexp.MustCompile(regexFilesystemReadOnly)
	compiledNVMePathFailure    = regexp.MustCompile(regexNVMePathFailure)
)

// Returns true if the line indicates that the disk has no space left.
func HasNoSpaceLeft(line string) bool {
	if match := compiledNoSpaceLeft.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

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
		{check: HasNoSpaceLeft, eventName: eventNoSpaceLeft, regex: regexNoSpaceLeft, message: messageNoSpaceLeft},
		{check: HasRAIDArrayFailure, eventName: eventRAIDArrayFailure, regex: regexRAIDArrayFailure, message: messageRAIDArrayFailure},
		{check: HasFilesystemReadOnly, eventName: eventFilesystemReadOnly, regex: regexFilesystemReadOnly, message: messageFilesystemReadOnly},
		{check: HasNVMePathFailure, eventName: eventNVMePathFailure, regex: regexNVMePathFailure, message: messageNVMePathFailure},
	}
}
