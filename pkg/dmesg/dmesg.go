// Package dmesg provides the functionality to poll the dmesg log.
package dmesg

import (
	"bytes"
	"errors"
	"time"
)

const (
	isoTimeFormat      = "2006-01-02T15:04:05,999999-07:00"
	shortIsoTimeFormat = "2006-01-02T15:04:05-0700"
)

var (
	isoTimeFormatN      = len(isoTimeFormat)
	shortIsoTimeFormatN = len(shortIsoTimeFormat)
)

// ParseDmesgTimeISO parses the timestamp from "dmesg --time-format=iso" output lines.
// ref.
// "The definition of the iso timestamp is: YYYY-MM-DD<T>HH:MM:SS,<microseconds>←+><timezone offset from UTC>."
func ParseDmesgTimeISO(line []byte) (time.Time, []byte, error) {
	if len(line) < isoTimeFormatN {
		return time.Time{}, nil, errors.New("line is too short")
	}

	// Example input: 2024-11-15T12:02:03,561522+00:00
	parsedTime, err := time.Parse("2006-01-02T15:04:05,999999-07:00", string(line[:isoTimeFormatN]))
	if err != nil {
		return time.Time{}, nil, err
	}

	extractedLine := bytes.TrimSpace(line[isoTimeFormatN:])
	return parsedTime, extractedLine, nil
}

// ParseJournalctlTimeShortISO parses the timestamp from "journalctl -o short-iso" output lines.
func ParseJournalctlTimeShortISO(line []byte) (time.Time, []byte, error) {
	if len(line) < shortIsoTimeFormatN {
		return time.Time{}, nil, errors.New("line is too short")
	}

	// Example input: 2025-01-02T15:20:12+0800
	parsedTime, err := time.Parse("2006-01-02T15:04:05-0700", string(line[:shortIsoTimeFormatN]))
	if err != nil {
		return time.Time{}, nil, err
	}

	extractedLine := bytes.TrimSpace(line[shortIsoTimeFormatN:])
	return parsedTime, extractedLine, nil
}
