package packages

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPackageInfo(t *testing.T) {
	info := PackageInfo{
		Name:          "test-package",
		ScriptPath:    "/path/to/script",
		TargetVersion: "1.0.0",
		Dependency:    [][]string{{"dep1", "1.0.0"}, {"dep2", "2.0.0"}},
		TotalTime:     5 * time.Second,
	}

	// Test field values
	assert.Equal(t, "test-package", info.Name)
	assert.Equal(t, "/path/to/script", info.ScriptPath)
	assert.Equal(t, "1.0.0", info.TargetVersion)
	assert.Equal(t, [][]string{{"dep1", "1.0.0"}, {"dep2", "2.0.0"}}, info.Dependency)
	assert.Equal(t, 5*time.Second, info.TotalTime)
}

func TestPackageStatus(t *testing.T) {
	status := PackageStatus{
		Name:           "test-package",
		Skipped:        false,
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		TotalTime:      5 * time.Second,
		Status:         true,
		TargetVersion:  "1.0.0",
		CurrentVersion: "0.9.0",
		ScriptPath:     "/path/to/script",
		Dependency:     [][]string{{"dep1", "1.0.0"}, {"dep2", "2.0.0"}},
	}

	// Test field values
	assert.Equal(t, "test-package", status.Name)
	assert.False(t, status.Skipped)
	assert.True(t, status.IsInstalled)
	assert.False(t, status.Installing)
	assert.Equal(t, 100, status.Progress)
	assert.Equal(t, 5*time.Second, status.TotalTime)
	assert.True(t, status.Status)
	assert.Equal(t, "1.0.0", status.TargetVersion)
	assert.Equal(t, "0.9.0", status.CurrentVersion)
	assert.Equal(t, "/path/to/script", status.ScriptPath)
	assert.Equal(t, [][]string{{"dep1", "1.0.0"}, {"dep2", "2.0.0"}}, status.Dependency)
}

func TestPackageStatusesSorting(t *testing.T) {
	statuses := PackageStatuses{
		{
			Name: "c-package",
		},
		{
			Name: "a-package",
		},
		{
			Name: "b-package",
		},
	}

	// Test Len()
	assert.Equal(t, 3, statuses.Len())

	// Test initial order
	assert.Equal(t, "c-package", statuses[0].Name)
	assert.Equal(t, "a-package", statuses[1].Name)
	assert.Equal(t, "b-package", statuses[2].Name)

	// Test Swap()
	statuses.Swap(0, 1)
	assert.Equal(t, "a-package", statuses[0].Name)
	assert.Equal(t, "c-package", statuses[1].Name)
	assert.Equal(t, "b-package", statuses[2].Name)

	// Test Less()
	assert.True(t, statuses.Less(0, 1))  // "a-package" < "c-package"
	assert.False(t, statuses.Less(1, 2)) // "c-package" > "b-package"

	// Test sorting
	sort.Sort(statuses)
	assert.Equal(t, "a-package", statuses[0].Name)
	assert.Equal(t, "b-package", statuses[1].Name)
	assert.Equal(t, "c-package", statuses[2].Name)
}

func TestPackageStatusesEdgeCases(t *testing.T) {
	// Test empty list
	emptyStatuses := PackageStatuses{}
	assert.Equal(t, 0, emptyStatuses.Len())
	sort.Sort(emptyStatuses) // Should not panic

	// Test single item
	singleStatus := PackageStatuses{{Name: "single"}}
	assert.Equal(t, 1, singleStatus.Len())
	sort.Sort(singleStatus) // Should not panic
	assert.Equal(t, "single", singleStatus[0].Name)

	// Test duplicate names
	duplicateStatuses := PackageStatuses{
		{Name: "same-name"},
		{Name: "same-name"},
	}
	sort.Sort(duplicateStatuses) // Should not panic
	assert.Equal(t, "same-name", duplicateStatuses[0].Name)
	assert.Equal(t, "same-name", duplicateStatuses[1].Name)

	// Test special characters in names
	specialStatuses := PackageStatuses{
		{Name: "!special"},
		{Name: "@package"},
		{Name: "#test"},
	}
	sort.Sort(specialStatuses)
	assert.Equal(t, "!special", specialStatuses[0].Name)
	assert.Equal(t, "#test", specialStatuses[1].Name)
	assert.Equal(t, "@package", specialStatuses[2].Name)
}

func TestPackageStatusesJSON(t *testing.T) {
	// Create a test status with all fields populated
	status := PackageStatus{
		Name:           "test-package",
		Skipped:        false,
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		TotalTime:      5 * time.Second,
		Status:         true,
		TargetVersion:  "1.0.0",
		CurrentVersion: "0.9.0",
		ScriptPath:     "/path/to/script",
		Dependency:     [][]string{{"dep1", "1.0.0"}, {"dep2", "2.0.0"}},
	}

	// Verify JSON field tags
	assert.Equal(t, "name", getJSONTag(status, "Name"))
	assert.Equal(t, "skipped", getJSONTag(status, "Skipped"))
	assert.Equal(t, "is_installed", getJSONTag(status, "IsInstalled"))
	assert.Equal(t, "installing", getJSONTag(status, "Installing"))
	assert.Equal(t, "progress", getJSONTag(status, "Progress"))
	assert.Equal(t, "total_time", getJSONTag(status, "TotalTime"))
	assert.Equal(t, "status", getJSONTag(status, "Status"))
	assert.Equal(t, "target_version", getJSONTag(status, "TargetVersion"))
	assert.Equal(t, "current_version", getJSONTag(status, "CurrentVersion"))
	assert.Equal(t, "script_path", getJSONTag(status, "ScriptPath"))
	assert.Equal(t, "dependency", getJSONTag(status, "Dependency"))
}

// Helper function to get JSON tag from struct field
func getJSONTag(v interface{}, fieldName string) string {
	field, ok := reflect.TypeOf(v).FieldByName(fieldName)
	if !ok {
		return ""
	}
	tag := field.Tag.Get("json")
	if idx := strings.Index(tag, ","); idx != -1 {
		tag = tag[:idx]
	}
	return tag
}

func TestPackageStatuses_RenderTable(t *testing.T) {
	tests := []struct {
		name     string
		statuses PackageStatuses
		contains []string
	}{
		{
			name:     "empty package statuses",
			statuses: PackageStatuses{},
			contains: []string{
				"PACKAGE NAME",
				"STATUS",
				"PROGRESS",
				"VERSION",
				"TIME ELAPSED",
				"EST",
			},
		},
		{
			name: "single package installing",
			statuses: PackageStatuses{
				{
					Name:           "nvidia-driver",
					IsInstalled:    false,
					Installing:     true,
					Progress:       50,
					TotalTime:      10 * time.Minute,
					CurrentVersion: "525.89.02",
					TargetVersion:  "535.104.05",
				},
			},
			contains: []string{
				"nvidia-driver",
				"Installing",
				"[==========          ] 50%",
				"525.89.02 → 535.104.05",
			},
		},
		{
			name: "multiple packages with different states",
			statuses: PackageStatuses{
				{
					Name:           "cuda-toolkit",
					IsInstalled:    true,
					Installing:     false,
					Progress:       100,
					TotalTime:      5 * time.Minute,
					CurrentVersion: "12.1",
					TargetVersion:  "12.1",
				},
				{
					Name:           "nvidia-container-toolkit",
					IsInstalled:    false,
					Installing:     false,
					Progress:       0,
					TotalTime:      0,
					CurrentVersion: "",
					TargetVersion:  "1.14.3",
				},
			},
			contains: []string{
				"cuda-toolkit",
				"✅",
				"[====================] 100%",
				"12.1",
				"nvidia-container-toolkit",
				"Not Installed",
				"[                    ] 0%",
				"N/A",
			},
		},
		{
			name: "uninstalled package with non-zero progress should show 0%",
			statuses: PackageStatuses{
				{
					Name:           "nvidia-driver",
					IsInstalled:    false,
					Installing:     false,
					Progress:       100, // This should be overridden to 0%
					TotalTime:      10 * time.Minute,
					CurrentVersion: "",
					TargetVersion:  "570.158.01",
				},
				{
					Name:           "kubelet",
					IsInstalled:    false,
					Installing:     false,
					Progress:       50, // This should also be overridden to 0%
					TotalTime:      5 * time.Minute,
					CurrentVersion: "",
					TargetVersion:  "1.32.8",
				},
			},
			contains: []string{
				"nvidia-driver",
				"Not Installed",
				"[                    ] 0%", // Should show 0% not 100%
				"570.158.01",
				"kubelet",
				"Not Installed",
				"[                    ] 0%", // Should show 0% not 50%
				"1.32.8",
			},
		},
		{
			name: "skipped package should show skipped status",
			statuses: PackageStatuses{
				{
					Name:           "skipped-package",
					Skipped:        true,
					IsInstalled:    true,
					Installing:     false,
					Progress:       100,
					TotalTime:      5 * time.Minute,
					CurrentVersion: "1.0.0",
					TargetVersion:  "1.0.0",
				},
			},
			contains: []string{
				"skipped-package",
				"Skipped",
				"1.0.0",
			},
		},
		{
			name: "skipped takes precedence over installed",
			statuses: PackageStatuses{
				{
					Name:           "priority-test",
					Skipped:        true,
					IsInstalled:    true, // Even if installed, should show Skipped
					Installing:     false,
					Progress:       100,
					TotalTime:      5 * time.Minute,
					CurrentVersion: "2.0.0",
					TargetVersion:  "2.0.0",
				},
			},
			contains: []string{
				"priority-test",
				"Skipped",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			tt.statuses.RenderTable(&buf)
			output := buf.String()

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestCreateProgressBar(t *testing.T) {
	tests := []struct {
		name     string
		progress int
		width    int
		expected string
	}{
		{
			name:     "0% progress",
			progress: 0,
			width:    20,
			expected: "[                    ]",
		},
		{
			name:     "50% progress",
			progress: 50,
			width:    20,
			expected: "[==========          ]",
		},
		{
			name:     "100% progress",
			progress: 100,
			width:    20,
			expected: "[====================]",
		},
		{
			name:     "25% progress with 10 width",
			progress: 25,
			width:    10,
			expected: "[==        ]",
		},
		{
			name:     "negative progress (should be 0)",
			progress: -10,
			width:    20,
			expected: "[                    ]",
		},
		{
			name:     "over 100% progress (should be capped at 100)",
			progress: 150,
			width:    20,
			expected: "[====================]",
		},
		{
			name:     "33% progress",
			progress: 33,
			width:    20,
			expected: "[======              ]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createProgressBar(tt.progress, tt.width)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPackageStatuses_RenderTable_ProgressStates(t *testing.T) {
	// Test specific progress states and time calculations
	statuses := PackageStatuses{
		{
			Name:           "package-at-start",
			IsInstalled:    false,
			Installing:     true,
			Progress:       0,
			TotalTime:      10 * time.Minute,
			CurrentVersion: "1.0.0",
			TargetVersion:  "2.0.0",
		},
		{
			Name:           "package-in-progress",
			IsInstalled:    false,
			Installing:     true,
			Progress:       75,
			TotalTime:      20 * time.Minute,
			CurrentVersion: "1.0.0",
			TargetVersion:  "2.0.0",
		},
		{
			Name:           "package-completed",
			IsInstalled:    true,
			Installing:     false,
			Progress:       100,
			TotalTime:      15 * time.Minute,
			CurrentVersion: "2.0.0",
			TargetVersion:  "2.0.0",
		},
	}

	var buf strings.Builder
	statuses.RenderTable(&buf)
	output := buf.String()

	// Check for specific status indicators
	expectedStrings := []string{
		"package-at-start",
		"Not started",
		"package-in-progress",
		"Installing",
		"[===============     ] 75%",
		"package-completed",
		"✅",
		"Complete",
	}

	for _, expected := range expectedStrings {
		assert.Contains(t, output, expected)
	}
}
