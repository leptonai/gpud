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
