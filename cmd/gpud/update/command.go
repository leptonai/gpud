// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This file is based on https://github.com/tailscale/tailscale/blob/012933635b43ac41c8ff4340213bdae9abd6d059/clientupdate/clientupdate.go

package update

import (
	"fmt"

	"github.com/urfave/cli"

	pkgupdate "github.com/leptonai/gpud/pkg/update"
	"github.com/leptonai/gpud/version"
)

func Command(cliContext *cli.Context) error {
	ver := cliContext.String("next-version")
	if ver == "" {
		var err error
		ver, err = version.DetectLatestVersion()
		if err != nil {
			fmt.Printf("Failed to fetch latest version: %v\n", err)
			return err
		}
	}

	url := cliContext.String("url")
	if url == "" {
		url = version.DefaultURLPrefix
	}

	return pkgupdate.Update(ver, url)
}

func CommandCheck(cliContext *cli.Context) error {
	ver, err := version.DetectLatestVersion()
	if err != nil {
		fmt.Printf("failed to detect the latest version: %v\n", err)
		return err
	}

	fmt.Printf("latest version: %s\n", ver)
	return nil
}
