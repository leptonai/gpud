package common

type ToolOverwrites struct {
	InfinibandClassRootDir string `json:"infiniband_class_root_dir"`

	// ExcludedInfinibandDevices is a list of InfiniBand device names to exclude from monitoring.
	// Device names should be like "mlx5_0", "mlx5_1", etc. (not full paths).
	//
	// This is useful for excluding devices that have restricted Physical Functions (PFs)
	// and cause kernel errors (mlx5_cmd_out_err ACCESS_REG) when queried.
	// This is common on NVIDIA DGX, Umbriel, and GB200 systems with ConnectX-7 adapters
	// where some ports are managed by system firmware.
	//
	// Example: ["mlx5_0", "mlx5_1"]
	//
	// ref.
	// https://github.com/prometheus/node_exporter/issues/3434
	// https://github.com/leptonai/gpud/issues/1164
	ExcludedInfinibandDevices []string `json:"excluded_infiniband_devices"`
}
