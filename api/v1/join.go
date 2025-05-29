package v1

// JoinRequest is the request for the join request.
type JoinRequest struct {
	ID                 string `json:"id"`
	ClusterName        string `json:"cluster_name,omitempty"`
	PublicIP           string `json:"public_ip,omitempty"`
	PrivateIP          string `json:"private_ip,omitempty"`
	Provider           string `json:"provider,omitempty"`
	ProviderInstanceID string `json:"provider_instance_id,omitempty"`
	ProviderGPUShape   string `json:"provider_gpu_shape,omitempty"`
	TotalCPU           int64  `json:"total_cpu,omitempty"`
	NodeGroup          string `json:"node_group,omitempty"`
	ExtraInfo          string `json:"extra_info,omitempty"`
	Region             string `json:"region,omitempty"`
}

// JoinResponse is the response for the join request.
type JoinResponse struct {
	Error  string `json:"error"`
	Status string `json:"status"`
}
