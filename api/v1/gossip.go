package v1

// GossipRequest is the request for the gossip request.
type GossipRequest struct {
	MachineID     string   `json:"machineID"`
	DaemonVersion string   `json:"daemonVersion"`
	Components    []string `json:"components"`
}

// GossipResponse is the response for the gossip request.
type GossipResponse struct {
	Error  string `json:"error"`
	Status string `json:"status"`
}
