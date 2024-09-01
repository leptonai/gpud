package packages

type PackageInfo struct {
	Name          string
	ScriptPath    string
	TargetVersion string
	Dependency    [][]string
}

type PackageStatus struct {
	Name           string
	IsInstalled    bool
	Installing     bool
	Status         bool
	TargetVersion  string
	CurrentVersion string
	ScriptPath     string
	Dependency     [][]string
}
