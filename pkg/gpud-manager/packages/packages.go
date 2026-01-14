package packages

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
)

type PackageInfo struct {
	Name          string
	ScriptPath    string
	TargetVersion string
	Dependency    [][]string
	TotalTime     time.Duration
}

type PackageStatus struct {
	Name           string        `json:"name"`
	Skipped        bool          `json:"skipped"`
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

func (ps PackageStatuses) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Package Name", "Status", "Progress", "Version", "Time Elapsed", "Est. Time Left"})

	// Configure table style
	table.SetBorder(true)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)

	for _, status := range ps {
		// Determine status text
		statusText := "Not Installed"
		if status.Skipped {
			statusText = "Skipped"
		} else if status.IsInstalled {
			statusText = "✅"
		} else if status.Installing {
			statusText = "Installing"
		}

		// Create progress bar
		displayProgress := status.Progress
		if !status.IsInstalled && !status.Installing {
			displayProgress = 0
		}
		progressBar := createProgressBar(displayProgress, 20)
		progressText := fmt.Sprintf("%s %d%%", progressBar, displayProgress)

		// Calculate time elapsed and time left
		var timeElapsed, timeLeft string
		if status.TotalTime > 0 {
			elapsed := time.Duration(float64(status.TotalTime) * float64(status.Progress) / 100)
			timeElapsed = humanize.Time(time.Now().Add(-elapsed))

			if status.Progress < 100 && status.Progress > 0 {
				remaining := time.Duration(float64(status.TotalTime) * float64(100-status.Progress) / 100)
				timeLeft = humanize.Time(time.Now().Add(remaining))
			} else if status.Progress == 0 {
				timeLeft = "Not started"
			} else {
				timeLeft = "Complete"
			}
		} else {
			timeElapsed = "N/A"
			timeLeft = "N/A"
		}

		// Version info
		versionInfo := status.CurrentVersion
		if status.TargetVersion != "" && status.TargetVersion != status.CurrentVersion {
			versionInfo = fmt.Sprintf("%s → %s", status.CurrentVersion, status.TargetVersion)
		}

		table.Append([]string{
			status.Name,
			statusText,
			progressText,
			versionInfo,
			timeElapsed,
			timeLeft,
		})
	}

	table.Render()
}

// createProgressBar creates a visual progress bar
func createProgressBar(progress int, width int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}

	filled := int(float64(width) * float64(progress) / 100)
	empty := width - filled

	bar := "[" + strings.Repeat("=", filled) + strings.Repeat(" ", empty) + "]"
	return bar
}
