// Package latency contains logic for egress traffic from each device.
package latency

import (
	"io"

	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Latencies []Latency

func (l Latencies) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Provider", "Region Name", "Region Code", "Latency"})

	for _, latency := range l {
		table.Append([]string{
			latency.Provider,
			latency.RegionName,
			latency.RegionCode,
			latency.Latency.Duration.String(),
		})
	}

	table.Render()
}

// Latency measures the time it takes for a request to be sent to an edge server and back.
// It measures the egress latency from the perspective of the local device.
type Latency struct {
	// Defines the edge server provider type (e.g., tailscale DERP).
	Provider string `json:"provider"`

	// Region name of the edge server.
	RegionName string `json:"region_name"`

	// The region code of the edge server.
	// e.g., Named "us-east-1" to be consistent with other cloud providers.
	RegionCode string `json:"region_code"`

	// Latency of the edge server.
	// It is a time that the request takes to be sent to the edge server and back.
	Latency metav1.Duration `json:"latency"`

	// Latency converted to milliseconds.
	LatencyMilliseconds int64 `json:"latency_milliseconds"`
}
