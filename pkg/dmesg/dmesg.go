// Package dmesg provides the functionality to poll the dmesg log.
package dmesg

import (
	"errors"
	"regexp"
	"time"
)

var regexForDmesgTime = regexp.MustCompile(`^\[([^\]]+)\]`)

// Parses the timestamp from "dmesg --ctime" output lines.
// Returns a zero time if the timestamp is not found or the timestamp is invalid.
// Returns an error if the timestamp is not found or the timestamp is invalid.
func ParseCtimeWithError(line []byte) (time.Time, error) {
	matches := regexForDmesgTime.FindStringSubmatch(string(line))
	if len(matches) == 0 {
		return time.Time{}, errors.New("no timestamp matches found")
	}

	s := matches[1]
	timestamp, err := time.Parse("Mon Jan 2 15:04:05 2006", s)
	if err != nil {
		return time.Time{}, err
	}
	return timestamp, nil
}
