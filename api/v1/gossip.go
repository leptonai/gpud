package v1

// GossipRequest is the request for the gossip request.
type GossipRequest struct {
	MachineID   string      `json:"machineID"`
	MachineInfo MachineInfo `json:"machineInfo"`
}

// GossipResponse is the response for the gossip request.
type GossipResponse struct {
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}
