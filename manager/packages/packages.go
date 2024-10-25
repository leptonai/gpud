package packages

import "time"

type PackageInfo struct {
	Name          string
	ScriptPath    string
	TargetVersion string
	Dependency    [][]string
	TotalTime     time.Duration
}

type PackageStatus struct {
	Name           string        `json:"name"`
	IsInstalled    bool          `json:"is_installed"`
	Installing     bool          `json:"installing"`
	Progress       int           `json:"progress"`
	TotalTime      time.Duration `json:"total_time"`
	Status         bool          `json:"status"`
	TargetVersion  string        `json:"target_version"`
	CurrentVersion string        `json:"current_version"`
	ScriptPath     string        `json:"script_path"`
	Dependency     [][]string    `json:"dependency"`
}

type PackageStatuses []PackageStatus

func (a PackageStatuses) Len() int { return len(a) }

func (a PackageStatuses) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

func (a PackageStatuses) Less(i, j int) bool { return a[i].Name < a[j].Name }
