package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	customplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	"github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSaveSpecs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		initialSpecs *customplugins.Specs
		specsToSave  customplugins.Specs
		expectSaved  bool
		expectError  bool
	}{
		{
			name:         "save specs when none exist",
			initialSpecs: nil,
			specsToSave: customplugins.Specs{
				{
					PluginName: "test-plugin",
					Type:       "component",
					RunMode:    "auto",
					Timeout:    metav1.Duration{Duration: time.Minute},
					Interval:   metav1.Duration{Duration: 30 * time.Second},
				},
			},
			expectSaved: true,
			expectError: false,
		},
		{
			name: "save specs when different from existing",
			initialSpecs: &customplugins.Specs{
				{
					PluginName: "old-plugin",
					Type:       "init",
					RunMode:    "auto",
				},
			},
			specsToSave: customplugins.Specs{
				{
					PluginName: "new-plugin",
					Type:       "component",
					RunMode:    "manual",
					Timeout:    metav1.Duration{Duration: 2 * time.Minute},
					Interval:   metav1.Duration{Duration: time.Minute},
				},
			},
			expectSaved: true,
			expectError: false,
		},
		{
			name: "do not save specs when same as existing",
			initialSpecs: &customplugins.Specs{
				{
					PluginName: "test-plugin",
					Type:       "component",
					RunMode:    "auto",
					Timeout:    metav1.Duration{Duration: time.Minute},
					Interval:   metav1.Duration{Duration: 30 * time.Second},
				},
			},
			specsToSave: customplugins.Specs{
				{
					PluginName: "test-plugin",
					Type:       "component",
					RunMode:    "auto",
					Timeout:    metav1.Duration{Duration: time.Minute},
					Interval:   metav1.Duration{Duration: 30 * time.Second},
				},
			},
			expectSaved: false,
			expectError: false,
		},
		{
			name:         "save empty specs",
			initialSpecs: nil,
			specsToSave:  customplugins.Specs{},
			expectSaved:  true,
			expectError:  false,
		},
		{
			name:         "save specs with complex configuration",
			initialSpecs: nil,
			specsToSave: customplugins.Specs{
				{
					PluginName:    "complex-plugin",
					Type:          "component_list",
					RunMode:       "auto",
					Tags:          []string{"tag1", "tag2"},
					ComponentList: []string{"comp1", "comp2:param"},
					HealthStatePlugin: &customplugins.Plugin{
						Steps: []customplugins.Step{
							{
								Name: "health-check",
								RunBashScript: &customplugins.RunBashScript{
									ContentType: "plaintext",
									Script:      "echo 'healthy'",
								},
							},
						},
					},
					Timeout:  metav1.Duration{Duration: 5 * time.Minute},
					Interval: metav1.Duration{Duration: 2 * time.Minute},
				},
			},
			expectSaved: true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			// Create metadata table
			err := metadata.CreateTableMetadata(ctx, dbRW)
			require.NoError(t, err)

			// Set up initial specs if provided
			if tt.initialSpecs != nil {
				_, err := SaveSpecs(ctx, dbRW, *tt.initialSpecs)
				require.NoError(t, err)
			}

			// Get initial metadata value for comparison
			initialValue, err := metadata.ReadMetadata(ctx, dbRO, metadata.MetadataKeyPluginsSpec)
			require.NoError(t, err)

			// Save the specs
			saved, err := SaveSpecs(ctx, dbRW, tt.specsToSave)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectSaved, saved, "SaveSpecs return value should match expected")

			// Check if metadata was actually saved/updated
			finalValue, err := metadata.ReadMetadata(ctx, dbRO, metadata.MetadataKeyPluginsSpec)
			require.NoError(t, err)

			if tt.expectSaved {
				assert.NotEqual(t, initialValue, finalValue, "metadata should have been updated")

				// Verify we can load the saved specs back
				loadedSpecs, err := LoadSpecs(ctx, dbRO)
				require.NoError(t, err)
				require.NotNil(t, loadedSpecs)
				assert.Equal(t, tt.specsToSave, loadedSpecs)
			} else {
				assert.Equal(t, initialValue, finalValue, "metadata should not have been updated")
			}
		})
	}
}

func TestSaveSpecsErrors(t *testing.T) {
	t.Parallel()

	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create metadata table
	err := metadata.CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test with canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Immediately cancel

	specs := customplugins.Specs{
		{
			PluginName: "test-plugin",
			Type:       "component",
			RunMode:    "auto",
		},
	}

	saved, err := SaveSpecs(canceledCtx, dbRW, specs)
	assert.Error(t, err)
	assert.False(t, saved, "SaveSpecs should return false when there's an error")
}

func TestSaveSpecsBooleanReturns(t *testing.T) {
	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Create metadata table
	err := metadata.CreateTableMetadata(ctx, dbRW)
	require.NoError(t, err)

	// Test 1: First save should return true (new record)
	specs1 := customplugins.Specs{
		{
			PluginName: "test-plugin-1",
			Type:       "component",
			RunMode:    "auto",
			Timeout:    metav1.Duration{Duration: time.Minute},
		},
	}

	saved, err := SaveSpecs(ctx, dbRW, specs1)
	require.NoError(t, err)
	assert.True(t, saved, "First save should return true")

	// Verify data was actually saved
	loadedSpecs, err := LoadSpecs(ctx, dbRO)
	require.NoError(t, err)
	assert.Equal(t, specs1, loadedSpecs)

	// Test 2: Saving identical specs should return false (no update needed)
	saved, err = SaveSpecs(ctx, dbRW, specs1)
	require.NoError(t, err)
	assert.False(t, saved, "Saving identical specs should return false")

	// Test 3: Saving different specs should return true (update)
	specs2 := customplugins.Specs{
		{
			PluginName: "test-plugin-2",
			Type:       "component",
			RunMode:    "manual",
			Timeout:    metav1.Duration{Duration: 2 * time.Minute},
		},
	}

	saved, err = SaveSpecs(ctx, dbRW, specs2)
	require.NoError(t, err)
	assert.True(t, saved, "Saving different specs should return true")

	// Verify the new data was saved
	loadedSpecs, err = LoadSpecs(ctx, dbRO)
	require.NoError(t, err)
	assert.Equal(t, specs2, loadedSpecs)

	// Test 4: Saving empty specs should return true (update to empty)
	emptySpecs := customplugins.Specs{}
	saved, err = SaveSpecs(ctx, dbRW, emptySpecs)
	require.NoError(t, err)
	assert.True(t, saved, "Saving empty specs should return true")

	// Test 5: Saving empty specs again should return false (no change)
	saved, err = SaveSpecs(ctx, dbRW, emptySpecs)
	require.NoError(t, err)
	assert.False(t, saved, "Saving same empty specs should return false")

	// Test 6: Saving nil specs should return true when empty exists (different JSON)
	saved, err = SaveSpecs(ctx, dbRW, nil)
	require.NoError(t, err)
	assert.True(t, saved, "Saving nil specs when empty already exists should return true (different JSON)")

	// Test 7: Update from nil back to non-empty should return true
	specs3 := customplugins.Specs{
		{
			PluginName: "test-plugin-3",
			Type:       "init",
			RunMode:    "auto",
		},
	}

	saved, err = SaveSpecs(ctx, dbRW, specs3)
	require.NoError(t, err)
	assert.True(t, saved, "Updating from nil to non-empty should return true")
}

func TestLoadSpecs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupSpecs   *customplugins.Specs
		setupRawData *string // for testing invalid JSON
		expectSpecs  *customplugins.Specs
		expectError  bool
	}{
		{
			name:        "load when no specs exist",
			setupSpecs:  nil,
			expectSpecs: nil,
			expectError: false,
		},
		{
			name: "load existing specs",
			setupSpecs: &customplugins.Specs{
				{
					PluginName: "test-plugin",
					Type:       "component",
					RunMode:    "auto",
					Tags:       []string{"tag1"},
					Timeout:    metav1.Duration{Duration: time.Minute},
					Interval:   metav1.Duration{Duration: 30 * time.Second},
				},
			},
			expectSpecs: &customplugins.Specs{
				{
					PluginName: "test-plugin",
					Type:       "component",
					RunMode:    "auto",
					Tags:       []string{"tag1"},
					Timeout:    metav1.Duration{Duration: time.Minute},
					Interval:   metav1.Duration{Duration: 30 * time.Second},
				},
			},
			expectError: false,
		},
		{
			name:        "load empty specs",
			setupSpecs:  &customplugins.Specs{},
			expectSpecs: &customplugins.Specs{},
			expectError: false,
		},
		{
			name: "load complex specs",
			setupSpecs: &customplugins.Specs{
				{
					PluginName:        "complex-plugin",
					Type:              "component_list",
					RunMode:           "manual",
					Tags:              []string{"tag1", "tag2"},
					ComponentList:     []string{"comp1", "comp2:param"},
					ComponentListFile: "/path/to/file",
					HealthStatePlugin: &customplugins.Plugin{
						Steps: []customplugins.Step{
							{
								Name: "step1",
								RunBashScript: &customplugins.RunBashScript{
									ContentType: "base64",
									Script:      "ZWNobyBoZWxsbw==",
								},
							},
						},
						Parser: &customplugins.PluginOutputParseConfig{
							JSONPaths: []customplugins.JSONPath{
								{
									Query: "$.result",
									Field: "status",
								},
							},
						},
					},
					Timeout:  metav1.Duration{Duration: 10 * time.Minute},
					Interval: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
			expectSpecs: &customplugins.Specs{
				{
					PluginName:        "complex-plugin",
					Type:              "component_list",
					RunMode:           "manual",
					Tags:              []string{"tag1", "tag2"},
					ComponentList:     []string{"comp1", "comp2:param"},
					ComponentListFile: "/path/to/file",
					HealthStatePlugin: &customplugins.Plugin{
						Steps: []customplugins.Step{
							{
								Name: "step1",
								RunBashScript: &customplugins.RunBashScript{
									ContentType: "base64",
									Script:      "ZWNobyBoZWxsbw==",
								},
							},
						},
						Parser: &customplugins.PluginOutputParseConfig{
							JSONPaths: []customplugins.JSONPath{
								{
									Query: "$.result",
									Field: "status",
								},
							},
						},
					},
					Timeout:  metav1.Duration{Duration: 10 * time.Minute},
					Interval: metav1.Duration{Duration: 5 * time.Minute},
				},
			},
			expectError: false,
		},
		{
			name:         "load invalid JSON",
			setupRawData: stringPtr("{invalid json"),
			expectSpecs:  nil,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			// Create metadata table
			err := metadata.CreateTableMetadata(ctx, dbRW)
			require.NoError(t, err)

			// Set up test data
			if tt.setupSpecs != nil {
				_, err := SaveSpecs(ctx, dbRW, *tt.setupSpecs)
				require.NoError(t, err)
			} else if tt.setupRawData != nil {
				err := metadata.SetMetadata(ctx, dbRW, metadata.MetadataKeyPluginsSpec, *tt.setupRawData)
				require.NoError(t, err)
			}

			// Load specs
			specs, err := LoadSpecs(ctx, dbRO)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, specs)
				return
			}

			require.NoError(t, err)

			if tt.expectSpecs == nil {
				assert.Nil(t, specs)
			} else {
				require.NotNil(t, specs)
				assert.Equal(t, *tt.expectSpecs, specs)
			}
		})
	}
}

func TestLoadSpecsErrors(t *testing.T) {
	t.Parallel()

	_, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	// Test with canceled context
	canceledCtx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc() // Immediately cancel

	specs, err := LoadSpecs(canceledCtx, dbRO)
	assert.Error(t, err)
	assert.Nil(t, specs)
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

func TestSaveSpecsEdgeCases(t *testing.T) {
	t.Parallel()

	// Test with nil specs vs empty specs behavior
	t.Run("nil vs empty specs", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// Create metadata table
		err := metadata.CreateTableMetadata(ctx, dbRW)
		require.NoError(t, err)

		// Save nil specs first
		saved, err := SaveSpecs(ctx, dbRW, nil)
		require.NoError(t, err)
		assert.True(t, saved, "First save of nil specs should return true")

		// Save empty specs (should NOT be equivalent to nil - different JSON)
		emptySpecs := customplugins.Specs{}
		saved, err = SaveSpecs(ctx, dbRW, emptySpecs)
		require.NoError(t, err)
		assert.True(t, saved, "Empty specs should NOT be equivalent to nil specs (different JSON)")

		// Verify empty specs are loaded correctly
		loadedSpecs, err := LoadSpecs(ctx, dbRO)
		require.NoError(t, err)
		assert.Equal(t, customplugins.Specs{}, loadedSpecs)

		// Save empty specs again - should return false now
		saved, err = SaveSpecs(ctx, dbRW, emptySpecs)
		require.NoError(t, err)
		assert.False(t, saved, "Saving same empty specs again should return false")

		// Save nil specs again - should return true (different from empty)
		saved, err = SaveSpecs(ctx, dbRW, nil)
		require.NoError(t, err)
		assert.True(t, saved, "Saving nil specs after empty should return true (different JSON)")
	})

	// Test with complex specs containing all fields
	t.Run("complex specs with all fields", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// Create metadata table
		err := metadata.CreateTableMetadata(ctx, dbRW)
		require.NoError(t, err)

		complexSpecs := customplugins.Specs{
			{
				PluginName:        "complex-plugin",
				Type:              "component_list",
				RunMode:           "auto",
				Tags:              []string{"tag1", "tag2", "tag3"},
				ComponentList:     []string{"comp1", "comp2:param1", "comp3:param2:value"},
				ComponentListFile: "/path/to/component/list",
				HealthStatePlugin: &customplugins.Plugin{
					Steps: []customplugins.Step{
						{
							Name: "step1",
							RunBashScript: &customplugins.RunBashScript{
								ContentType: "base64",
								Script:      "ZWNobyBoZWxsbw==",
							},
						},
						{
							Name: "step2",
							RunBashScript: &customplugins.RunBashScript{
								ContentType: "plaintext",
								Script:      "echo 'world'",
							},
						},
					},
					Parser: &customplugins.PluginOutputParseConfig{
						JSONPaths: []customplugins.JSONPath{
							{
								Query: "$.status",
								Field: "health",
							},
							{
								Query: "$.error",
								Field: "error_msg",
							},
						},
					},
				},
				Timeout:  metav1.Duration{Duration: 15 * time.Minute},
				Interval: metav1.Duration{Duration: 5 * time.Minute},
			},
			{
				PluginName: "simple-plugin",
				Type:       "init",
				RunMode:    "manual",
				Tags:       []string{"simple"},
				Timeout:    metav1.Duration{Duration: 30 * time.Second},
			},
		}

		saved, err := SaveSpecs(ctx, dbRW, complexSpecs)
		require.NoError(t, err)
		assert.True(t, saved, "Saving complex specs should return true")

		// Verify complex specs can be loaded back correctly
		loadedSpecs, err := LoadSpecs(ctx, dbRO)
		require.NoError(t, err)
		assert.Equal(t, complexSpecs, loadedSpecs)

		// Save the same complex specs again
		saved, err = SaveSpecs(ctx, dbRW, complexSpecs)
		require.NoError(t, err)
		assert.False(t, saved, "Saving identical complex specs should return false")
	})

	// Test updating specs with minor differences
	t.Run("minor spec differences", func(t *testing.T) {
		dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// Create metadata table
		err := metadata.CreateTableMetadata(ctx, dbRW)
		require.NoError(t, err)

		originalSpecs := customplugins.Specs{
			{
				PluginName: "test-plugin",
				Type:       "component",
				RunMode:    "auto",
				Timeout:    metav1.Duration{Duration: time.Minute},
				Interval:   metav1.Duration{Duration: 30 * time.Second},
			},
		}

		saved, err := SaveSpecs(ctx, dbRW, originalSpecs)
		require.NoError(t, err)
		assert.True(t, saved, "Saving original specs should return true")

		// Change only the timeout
		modifiedSpecs := customplugins.Specs{
			{
				PluginName: "test-plugin",
				Type:       "component",
				RunMode:    "auto",
				Timeout:    metav1.Duration{Duration: 2 * time.Minute}, // Changed
				Interval:   metav1.Duration{Duration: 30 * time.Second},
			},
		}

		saved, err = SaveSpecs(ctx, dbRW, modifiedSpecs)
		require.NoError(t, err)
		assert.True(t, saved, "Saving specs with timeout change should return true")

		// Verify the timeout change was saved
		loadedSpecs, err := LoadSpecs(ctx, dbRO)
		require.NoError(t, err)
		assert.Equal(t, modifiedSpecs, loadedSpecs)

		// Change only the run mode
		modeModifiedSpecs := customplugins.Specs{
			{
				PluginName: "test-plugin",
				Type:       "component",
				RunMode:    "manual", // Changed
				Timeout:    metav1.Duration{Duration: 2 * time.Minute},
				Interval:   metav1.Duration{Duration: 30 * time.Second},
			},
		}

		saved, err = SaveSpecs(ctx, dbRW, modeModifiedSpecs)
		require.NoError(t, err)
		assert.True(t, saved, "Saving specs with run mode change should return true")

		// Verify the run mode change was saved
		loadedSpecs, err = LoadSpecs(ctx, dbRO)
		require.NoError(t, err)
		assert.Equal(t, modeModifiedSpecs, loadedSpecs)
	})
}

func TestSaveSpecsDatabaseErrors(t *testing.T) {
	t.Parallel()

	// Test with closed database connection
	t.Run("closed database", func(t *testing.T) {
		dbRW, _, cleanup := sqlite.OpenTestDB(t)
		cleanup() // Close the database immediately

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		specs := customplugins.Specs{
			{
				PluginName: "test-plugin",
				Type:       "component",
				RunMode:    "auto",
			},
		}

		saved, err := SaveSpecs(ctx, dbRW, specs)
		assert.Error(t, err)
		assert.False(t, saved, "SaveSpecs should return false when database is closed")
	})

	// Test with invalid database (no metadata table)
	t.Run("no metadata table", func(t *testing.T) {
		dbRW, _, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		// Don't create metadata table

		specs := customplugins.Specs{
			{
				PluginName: "test-plugin",
				Type:       "component",
				RunMode:    "auto",
			},
		}

		saved, err := SaveSpecs(ctx, dbRW, specs)
		assert.Error(t, err)
		assert.False(t, saved, "SaveSpecs should return false when metadata table doesn't exist")
	})
}
