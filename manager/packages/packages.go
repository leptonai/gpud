package packages

type PackageInfo struct {
	Name          string
	ScriptPath    string
	TargetVersion string
	Dependency    [][]string
}

type PackageStatus struct {
	Name           string     `json:"name"`
	IsInstalled    bool       `json:"is_installed"`
	Installing     bool       `json:"installing"`
	Status         bool       `json:"status"`
	TargetVersion  string     `json:"target_version"`
	CurrentVersion string     `json:"current_version"`
	ScriptPath     string     `json:"script_path"`
	Dependency     [][]string `json:"dependency"`
}
