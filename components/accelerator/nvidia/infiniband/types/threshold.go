package types //nolint:revive // The directory name is intentionally generic for shared InfiniBand types.

// ExpectedPortStates configures the expected state of the ports.
type ExpectedPortStates struct {
	// Enabled distinguishes disabled validation from a request for a product default.
	// A nil value preserves the legacy behavior of zero thresholds.
	Enabled *bool `json:"enabled,omitempty"`

	// The minimum number of ports.
	// If not set, it defaults to 0.
	AtLeastPorts int `json:"at_least_ports"`

	// The expected rate in Gb/sec.
	// If not set, it defaults to 0.
	AtLeastRate int `json:"at_least_rate"`
}

// IsZero returns true if the expected port states are not set.
func (eps *ExpectedPortStates) IsZero() bool {
	if eps == nil {
		return true
	}
	return eps.AtLeastPorts <= 0 || eps.AtLeastRate <= 0
}
