package controllers

import (
	"testing"
	"time"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/stretchr/testify/assert"
)

func TestPackageController_SetAndGetMethods(t *testing.T) {
	tests := []struct {
		name     string
		pkg      string
		testFunc func(t *testing.T, c *PackageController)
	}{
		{
			name: "test version operations",
			pkg:  "test-pkg",
			testFunc: func(t *testing.T, c *PackageController) {
				// Test with non-existent package
				c.setCurrentVersion("non-existent", "1.0.0")
				assert.Equal(t, "", c.getCurrentVersion("non-existent"))

				// Test with existing package
				c.setCurrentVersion("test-pkg", "1.0.0")
				assert.Equal(t, "1.0.0", c.getCurrentVersion("test-pkg"))
			},
		},
		{
			name: "test progress operations",
			pkg:  "test-pkg",
			testFunc: func(t *testing.T, c *PackageController) {
				// Test with non-existent package
				c.setProgress("non-existent", 50)
				status := c.getPackageStatus("non-existent")
				assert.Nil(t, status)

				// Test with existing package
				c.setProgress("test-pkg", 50)
				status = c.getPackageStatus("test-pkg")
				assert.Equal(t, 50, status.Progress)
			},
		},
		{
			name: "test installing progress operations",
			pkg:  "test-pkg",
			testFunc: func(t *testing.T, c *PackageController) {
				// Test with non-existent package
				c.setInstallingProgress("non-existent", true, 50)
				status := c.getPackageStatus("non-existent")
				assert.Nil(t, status)

				// Test with existing package
				c.setInstallingProgress("test-pkg", true, 50)
				status = c.getPackageStatus("test-pkg")
				assert.True(t, status.Installing)
				assert.Equal(t, 50, status.Progress)
			},
		},
		{
			name: "test installed progress operations",
			pkg:  "test-pkg",
			testFunc: func(t *testing.T, c *PackageController) {
				// Test with non-existent package
				c.setInstalledProgress("non-existent", true, 100)
				assert.False(t, c.getIsInstalled("non-existent"))

				// Test with existing package
				c.setInstalledProgress("test-pkg", true, 100)
				assert.True(t, c.getIsInstalled("test-pkg"))
				status := c.getPackageStatus("test-pkg")
				assert.Equal(t, 100, status.Progress)
			},
		},
		{
			name: "test status operations",
			pkg:  "test-pkg",
			testFunc: func(t *testing.T, c *PackageController) {
				// Test with non-existent package
				c.setStatus("non-existent", true)
				status := c.getPackageStatus("non-existent")
				assert.Nil(t, status)

				// Test with existing package
				c.setStatus("test-pkg", true)
				status = c.getPackageStatus("test-pkg")
				assert.True(t, status.Status)
			},
		},
		{
			name: "test total time operations",
			pkg:  "test-pkg",
			testFunc: func(t *testing.T, c *PackageController) {
				// Test with non-existent package
				assert.Equal(t, time.Duration(0), c.getTotalTime("non-existent"))

				// Test with existing package
				expectedTime := 5 * time.Minute
				c.packageStatus["test-pkg"].TotalTime = expectedTime
				assert.Equal(t, expectedTime, c.getTotalTime("test-pkg"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewPackageController(make(chan packages.PackageInfo))
			// Initialize test package
			c.packageStatus[tt.pkg] = &packages.PackageStatus{
				Name: tt.pkg,
			}
			tt.testFunc(t, c)
		})
	}
}

func TestPackageController_GetPackageStatus(t *testing.T) {
	c := NewPackageController(make(chan packages.PackageInfo))

	// Test with non-existent package
	status := c.getPackageStatus("non-existent")
	assert.Nil(t, status)

	// Test with existing package
	expectedStatus := &packages.PackageStatus{
		Name:           "test-pkg",
		IsInstalled:    true,
		Installing:     false,
		Progress:       100,
		Status:         true,
		TargetVersion:  "1.0.0",
		CurrentVersion: "1.0.0",
	}
	c.packageStatus["test-pkg"] = expectedStatus

	status = c.getPackageStatus("test-pkg")
	assert.Equal(t, expectedStatus, status)
}
