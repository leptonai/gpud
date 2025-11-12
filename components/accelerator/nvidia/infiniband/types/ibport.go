// Package types contains shared types for the infiniband package to avoid import cycles.
package types

import "strings"

// IBPort is the port of the IB card.
type IBPort struct {
	// Device is the name of the IB port (e.g., mlx5_1).
	Device string `json:"device,omitempty"`
	// Port is the port number of the IB port (e.g., 1).
	Port uint `json:"port,omitempty"`
	// State is the state of the IB port (e.g., "Active", "Down")
	State string `json:"state,omitempty"`
	// PhysicalState is the physical state of the IB port (e.g., "LinkUp", "Disabled", "Polling")
	PhysicalState string `json:"physical_state,omitempty"`
	// RateGBSec is the rate in GB/s (e.g., 400).
	RateGBSec int `json:"rate_gb_sec,omitempty"`
	// LinkLayer is the link layer of the IB port (e.g., "Infiniband", "Ethernet")
	LinkLayer string `json:"link_layer,omitempty"`

	// TotalLinkDowned from counters/link_downed - Number of times the link has gone down due to error thresholds being exceeded.
	// A high value indicates link instability and potential hardware or cabling issues.
	// "Total number of times the Port Training state machine has failed the link error recovery process and downed the link."
	//
	// IB port flap when a port is down and back to active for the last 4-minute.
	// If [IBPort.TotalLinkDowned] increments and the current port state is "Active",
	// then we mark the port as "flapping"
	TotalLinkDowned uint64 `json:"total_link_downed"`
}

func (p IBPort) IsIBPort() bool {
	return strings.EqualFold(p.LinkLayer, "infiniband")
}
