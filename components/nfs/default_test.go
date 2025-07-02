package nfs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

func TestDefaultConfig(t *testing.T) {
	t.Run("get default config when not set", func(t *testing.T) {
		// Reset to ensure clean state
		SetDefaultConfigs(pkgnfschecker.Configs{})

		configs := GetDefaultConfigs()
		assert.Empty(t, configs)
	})

	t.Run("set and get default config", func(t *testing.T) {
		tempDir := t.TempDir()

		expectedConfig := pkgnfschecker.Config{
			VolumePath:   tempDir,
			FileContents: "test-content",
		}

		SetDefaultConfigs(pkgnfschecker.Configs{expectedConfig})
		actualConfigs := GetDefaultConfigs()

		assert.Len(t, actualConfigs, 1)
		actualConfig := actualConfigs[0]
		assert.Equal(t, expectedConfig.VolumePath, actualConfig.VolumePath)
		assert.Equal(t, expectedConfig.FileContents, actualConfig.FileContents)
	})

	t.Run("set config multiple times", func(t *testing.T) {
		tempDir1 := t.TempDir()
		tempDir2 := t.TempDir()

		// Set first config
		config1 := pkgnfschecker.Config{
			VolumePath:   tempDir1,
			FileContents: "content-1",
		}
		SetDefaultConfigs(pkgnfschecker.Configs{config1})

		retrieved1 := GetDefaultConfigs()
		assert.Len(t, retrieved1, 1)
		assert.Equal(t, config1.VolumePath, retrieved1[0].VolumePath)
		assert.Equal(t, config1.FileContents, retrieved1[0].FileContents)

		// Set second config (should overwrite)
		config2 := pkgnfschecker.Config{
			VolumePath:   tempDir2,
			FileContents: "content-2",
		}
		SetDefaultConfigs(pkgnfschecker.Configs{config2})

		retrieved2 := GetDefaultConfigs()
		assert.Len(t, retrieved2, 1)
		assert.Equal(t, config2.VolumePath, retrieved2[0].VolumePath)
		assert.Equal(t, config2.FileContents, retrieved2[0].FileContents)

		// Should not match the first config anymore
		assert.NotEqual(t, config1.VolumePath, retrieved2[0].VolumePath)
		assert.NotEqual(t, config1.FileContents, retrieved2[0].FileContents)
	})

	t.Run("concurrent access", func(t *testing.T) {
		tempDir := t.TempDir()

		config := pkgnfschecker.Config{
			VolumePath:   tempDir,
			FileContents: "concurrent-test",
		}

		// Set the config
		SetDefaultConfigs(pkgnfschecker.Configs{config})

		// Test concurrent reads
		const numGoroutines = 10
		results := make(chan pkgnfschecker.Configs, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				results <- GetDefaultConfigs()
			}()
		}

		// Collect all results
		for i := 0; i < numGoroutines; i++ {
			result := <-results
			assert.Len(t, result, 1)
			resultConfig := result[0]
			assert.Equal(t, config.VolumePath, resultConfig.VolumePath)
			assert.Equal(t, config.FileContents, resultConfig.FileContents)
		}
	})

	t.Run("set multiple configs", func(t *testing.T) {
		tempDir1 := t.TempDir()
		tempDir2 := t.TempDir()

		config1 := pkgnfschecker.Config{
			VolumePath:   tempDir1,
			FileContents: "content-1",
		}

		config2 := pkgnfschecker.Config{
			VolumePath:   tempDir2,
			FileContents: "content-2",
		}

		// Set multiple configs
		SetDefaultConfigs(pkgnfschecker.Configs{config1, config2})

		retrieved := GetDefaultConfigs()
		assert.Len(t, retrieved, 2)

		// Check first config
		assert.Equal(t, config1.VolumePath, retrieved[0].VolumePath)
		assert.Equal(t, config1.FileContents, retrieved[0].FileContents)

		// Check second config
		assert.Equal(t, config2.VolumePath, retrieved[1].VolumePath)
		assert.Equal(t, config2.FileContents, retrieved[1].FileContents)
	})
}
