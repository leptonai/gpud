package common

import "strings"

// ParseGPUUUIDs parses a comma-separated string of GPU UUIDs and returns a slice of trimmed UUIDs.
// Empty strings and strings with only spaces are filtered out.
func ParseGPUUUIDs(raw string) []string {
	uuids := make([]string, 0)
	for _, split := range strings.Split(raw, ",") {
		split = strings.TrimSpace(split)
		if split != "" {
			uuids = append(uuids, split)
		}
	}
	return uuids
}
