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
)

var (
	compiledNoSpaceLeft = regexp.MustCompile(regexNoSpaceLeft)
)

// Returns true if the line indicates that the disk has no space left.
func HasNoSpaceLeft(line string) bool {
	if match := compiledNoSpaceLeft.FindStringSubmatch(line); match != nil {
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
	}
}
