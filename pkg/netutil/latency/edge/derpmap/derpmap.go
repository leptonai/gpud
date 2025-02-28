// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package derpmap provides the tailscale derp map implementation.
package derpmap

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"tailscale.com/tailcfg"
)

// DefaultDERPMap is the default Tailscale DERP map.
// To avoid calling the public Tailscale endpoint, this is initialized from a local file.
var DefaultDERPMap tailcfg.DERPMap

//go:embed derpmap.json
var derpPMapRaw embed.FS

func init() {
	data, _ := derpPMapRaw.ReadFile("derpmap.json")
	err := json.Unmarshal(data, &DefaultDERPMap)
	if err != nil {
		panic(fmt.Errorf("failed to load DERP map: %v", err))
	}
}

const TailscaleDERPMapURL = "https://controlplane.tailscale.com/derpmap/default"

// DownloadTailcaleDERPMap downloads the official Tailscale public DERP map.
// ref. "prodDERPMap" in tailscale/tailscale/cmd/tailscale/cli/netcheck.go
func DownloadTailcaleDERPMap() (*tailcfg.DERPMap, error) {
	res, err := http.Get(TailscaleDERPMapURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var data tailcfg.DERPMap
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
