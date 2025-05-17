package v1

// JoinRequest is the request for the join request.
type JoinRequest struct {
	ID               string `json:"id"`
	ClusterName      string `json:"cluster_name,omitempty"`
	PublicIP         string `json:"public_ip"`
	Provider         string `json:"provider"`
	ProviderGPUShape string `json:"provider_gpu_shape,omitempty"`
	TotalCPU         int64  `json:"total_cpu"`
	NodeGroup        string `json:"node_group"`
	ExtraInfo        string `json:"extra_info"`
	Region           string `json:"region"`
	PrivateIP        string `json:"private_ip,omitempty"`
}

// JoinResponse is the response for the join request.
type JoinResponse struct {
	Error  string `json:"error"`
	Status string `json:"status"`
}
