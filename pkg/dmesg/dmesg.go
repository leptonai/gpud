// Package dmesg provides the functionality to poll the dmesg log.
package dmesg

import (
	"bytes"
	"errors"
	"regexp"
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

// ParseISOtimeWithError parses the timestamp from "dmesg --time-format=iso" output lines.
// ref.
// "The definition of the iso timestamp is: YYYY-MM-DD<T>HH:MM:SS,<microseconds>←+><timezone offset from UTC>."
func ParseISOtimeWithError(line []byte) (time.Time, []byte, error) {
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

// ParseShortISOtimeWithError parses the timestamp from "journalctl -o short-iso" output lines.
func ParseShortISOtimeWithError(line []byte) (time.Time, []byte, error) {
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

var regexForDmesgTime = regexp.MustCompile(`^\[([^\]]+)\]`)

// ParseCtimeWithError Parses the timestamp from "dmesg --ctime" output lines.
// Returns a zero time if the timestamp is not found or the timestamp is invalid.
// Returns an error if the timestamp is not found or the timestamp is invalid.
func ParseCtimeWithError(line []byte) (time.Time, []byte, error) {
	matches := regexForDmesgTime.FindStringSubmatch(string(line))
	if len(matches) == 0 {
		return time.Time{}, nil, errors.New("no timestamp matches found")
	}

	s := matches[1]
	timestamp, err := time.Parse(time.ANSIC, s)
	if err != nil {
		return time.Time{}, nil, err
	}
	return timestamp, line, nil
}
