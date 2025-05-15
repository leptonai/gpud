// Package release provides utilities for releasing new versions of gpud.
package release

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/pkg/release/distsign"
	"github.com/leptonai/gpud/version"
)

const defaultURLPrefix = "https://pkg.gpud.dev/"

func GetLatestVersion() (string, error) {
	track, err := sanitizeVersion(version.Version)
	if err != nil {
		return "", err
	}
	return GetLatestVersionByURL(defaultURLPrefix + track + "_latest.txt")
}

func GetLatestVersionByURL(url string) (string, error) {
	fetchedVer, err := distsign.Fetch(url, 100)
	if err != nil {
		fmt.Printf("Failed to fetch latest version: %v\n", err)
		return "", err
	}

	// trim whitespaces in case the version _latest.txt file included trailing spaces
	ver := string(bytes.TrimSpace(fetchedVer))
	fmt.Printf("automatically fetched the latest version: %s\n", ver)

	return ver, nil
}

func sanitizeVersion(v string) (string, error) {
	_, rest, ok := strings.Cut(v, ".")
	if !ok {
		return "", fmt.Errorf("malformed version %q", v)
	}
	minorStr, _, ok := strings.Cut(rest, ".")
	if !ok {
		return "", fmt.Errorf("malformed version %q", v)
	}
	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		return "", fmt.Errorf("malformed version %q", v)
	}
	if minor%2 == 0 {
		return "unstable", nil
	}
	return "stable", nil
}
