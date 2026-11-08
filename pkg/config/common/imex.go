package common

// IMEXConfig controls a read-only snapshot of the configured IMEX domain.
type IMEXConfig struct {
	// Required defaults to true for GB200/GB300. Job-scoped IMEX deployments
	// must set this false outside jobs or explicitly enable it for the job.
	Required *bool `json:"required,omitempty"`
	// Command is an argv prefix, for example ["nsenter","-t","1","-m","-n","--","nvidia-imex-ctl"].
	Command    []string `json:"command,omitempty"`
	ConfigFile string   `json:"config_file,omitempty"`
	// ServiceCommand optionally overrides systemctl is-active nvidia-imex.
	ServiceCommand []string `json:"service_command,omitempty"`
	// ExpectedAddresses optionally validates domain membership against inventory.
	ExpectedAddresses []string `json:"expected_addresses,omitempty"`
}
