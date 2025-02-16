// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This file is based on https://github.com/tailscale/tailscale/blob/012933635b43ac41c8ff4340213bdae9abd6d059/clientupdate/clientupdate.go

package command

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/pkg/third_party/tailscale/distsign"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
	"github.com/leptonai/gpud/version"

	"github.com/urfave/cli"
)

const defaultURLPrefix = "https://pkg.gpud.dev/"

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

func detectLatestVersion() (string, error) {
	track, err := versionToTrack(version.Version)
	if err != nil {
		return "", err
	}
	return detectLatestVersionByURL(defaultURLPrefix + track + "_latest.txt")
}

func detectLatestVersionByURL(url string) (string, error) {
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

func cmdUpdate(cliContext *cli.Context) error {
	ver := cliContext.String("next-version")
	if ver == "" {
		var err error
		ver, err = detectLatestVersion()
		if err != nil {
			fmt.Printf("Failed to fetch latest version: %v\n", err)
			return err
		}
	}

	url := cliContext.String("url")
	if url == "" {
		url = defaultURLPrefix
	}

	return pkgupdate.Update(ver, url)
}

func cmdUpdateCheck(cliContext *cli.Context) error {
	ver, err := detectLatestVersion()
	if err != nil {
		fmt.Printf("failed to detect the latest version: %v\n", err)
		return err
	}

	fmt.Printf("latest version: %s\n", ver)
	return nil
}
