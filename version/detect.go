package version

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/pkg/release/distsign"
)

const DefaultURLPrefix = "https://pkg.gpud.dev/"

func DetectLatestVersion() (string, error) {
	track, err := versionToTrack(Version)
	if err != nil {
		return "", err
	}
	return detectLatestVersionByURL(DefaultURLPrefix + track + "_latest.txt")
}

func versionToTrack(v string) (string, error) {
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

func detectLatestVersionByURL(url string) (string, error) {
	fetchedVer, err := distsign.Fetch(url, 100)
	if err != nil {
		return "", err
	}

	// trim whitespaces in case the version _latest.txt file included trailing spaces
	ver := string(bytes.TrimSpace(fetchedVer))
	fmt.Printf("automatically fetched the latest version: %s\n", ver)

	return ver, nil
}
