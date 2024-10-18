// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package edge

import (
	"context"
	"fmt"
	"sort"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/latency"
	"github.com/leptonai/gpud/pkg/latency/edge/derpmap"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"tailscale.com/net/netcheck"
	"tailscale.com/net/netmon"
	"tailscale.com/net/portmapper"
	"tailscale.com/types/logger"
)

const ProviderTailscaleDERP = "tailscale-derp"

// measureDERP measures the latencies from local to the global tailscale DERP nodes.
// ref. "tailscale netcheck" command https://github.com/tailscale/tailscale/blob/v1.76.1/cmd/tailscale/cli/netcheck.go.
func measureDERP(ctx context.Context, opts ...OpOption) (latency.Latencies, error) {
	op := new(Op)
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	log.Logger.Debugw("measuring latencies to public tailscale DERP nodes")

	logf := logger.Discard
	if op.verbose {
		logf = logger.WithPrefix(log.Logger.Printf, "derp: ")
	}

	netMon, err := netmon.New(logf)
	if err != nil {
		return nil, err
	}

	pm := portmapper.NewClient(logf, netMon, nil, nil, nil)
	defer pm.Close()

	c := &netcheck.Client{
		NetMon:      netMon,
		PortMapper:  pm,
		UseDNSCache: false, // always resolve, don't cache
		Logf:        logf,
		Verbose:     op.verbose,
	}

	dm := derpmap.DefaultDERPMap
	report, err := c.GetReport(ctx, &dm, nil)
	if err != nil {
		return nil, err
	}

	latencies := make(latency.Latencies, 0, len(report.RegionLatency))
	for regionID, dur := range report.RegionLatency {
		derpRegion, ok := dm.Regions[regionID]
		if !ok {
			return nil, fmt.Errorf("region %d not found in derpmap", regionID)
		}

		regionCode, ok := derpmap.GetRegionCode(derpRegion.RegionName)
		if !ok {
			return nil, fmt.Errorf("failed to get AWS region for %s", derpRegion.RegionName)
		}

		latencies = append(latencies, latency.Latency{
			Provider: ProviderTailscaleDERP,

			RegionName: derpRegion.RegionName,
			RegionCode: regionCode,

			Latency:             metav1.Duration{Duration: dur},
			LatencyMilliseconds: dur.Milliseconds(),
		})
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i].Latency.Duration < latencies[j].Latency.Duration
	})
	return latencies, nil
}
