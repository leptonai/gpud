package common

type ToolOverwrites struct {
	InfinibandClassRootDir string `json:"infiniband_class_root_dir"`

	// TODO: deprecate in favor of `InfinibandClassRootDir`.
	IbstatCommand string `json:"ibstat_command"`
}
