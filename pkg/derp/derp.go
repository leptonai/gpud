// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package derp provides a client for the Tailscale DERP (Designated Edge Router Protocol) service.
package derp

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/derp/derpmap"

	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"tailscale.com/net/netcheck"
	"tailscale.com/net/netmon"
	"tailscale.com/net/portmapper"
	"tailscale.com/types/logger"
)

type Op struct {
	verbose bool
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}

	return nil
}

func WithVerbose(verbose bool) OpOption {
	return func(op *Op) {
		op.verbose = verbose
	}
}

type Latencies []Latency

func (l Latencies) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Provider", "Region Name", "Latency"})

	for _, latency := range l {
		table.Append([]string{
			latency.Provider,
			latency.RegionName,
			latency.Latency.Duration.String(),
		})
	}

	table.Render()
}

type Latency struct {
	// DERP node provider name
	Provider string `json:"provider"`
	// DERP region name.
	RegionName string `json:"region_name"`
	// DERP latency.
	Latency metav1.Duration `json:"latency"`
	// LatencyMilliseconds is the latency in milliseconds.
	LatencyMilliseconds int64 `json:"latency_milliseconds"`
}

const (
	ProviderTailscale = "tailscale"
)

// MeasureLatencies measures the latencies from local to the global DERP nodes.
// ref. "tailscale netcheck" command https://github.com/tailscale/tailscale/blob/v1.76.1/cmd/tailscale/cli/netcheck.go.
func MeasureLatencies(ctx context.Context, opts ...OpOption) (Latencies, error) {
	op := new(Op)
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	log.Logger.Infow("measuring latencies to DERP nodes")

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

	latencies := make([]Latency, 0, len(report.RegionLatency))
	for regionID, dur := range report.RegionLatency {
		region, ok := dm.Regions[regionID]
		if !ok {
			return nil, fmt.Errorf("region %d not found in derpmap", regionID)
		}

		latencies = append(latencies, Latency{
			Provider:            ProviderTailscale,
			RegionName:          region.RegionName,
			Latency:             metav1.Duration{Duration: dur},
			LatencyMilliseconds: dur.Milliseconds(),
		})
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i].Latency.Duration < latencies[j].Latency.Duration
	})
	return latencies, nil
}
