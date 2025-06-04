package nfs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
			Dir:              tempDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 5,
		}

		SetDefaultConfigs(pkgnfschecker.Configs{expectedConfig})
		actualConfigs := GetDefaultConfigs()

		assert.Len(t, actualConfigs, 1)
		actualConfig := actualConfigs[0]
		assert.Equal(t, expectedConfig.Dir, actualConfig.Dir)
		assert.Equal(t, expectedConfig.FileContents, actualConfig.FileContents)
		assert.Equal(t, expectedConfig.TTLToDelete.Duration, actualConfig.TTLToDelete.Duration)
		assert.Equal(t, expectedConfig.NumExpectedFiles, actualConfig.NumExpectedFiles)
	})

	t.Run("set config multiple times", func(t *testing.T) {
		tempDir1 := t.TempDir()
		tempDir2 := t.TempDir()

		// Set first config
		config1 := pkgnfschecker.Config{
			Dir:              tempDir1,
			FileContents:     "content-1",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}
		SetDefaultConfigs(pkgnfschecker.Configs{config1})

		retrieved1 := GetDefaultConfigs()
		assert.Len(t, retrieved1, 1)
		assert.Equal(t, config1.Dir, retrieved1[0].Dir)
		assert.Equal(t, config1.FileContents, retrieved1[0].FileContents)

		// Set second config (should overwrite)
		config2 := pkgnfschecker.Config{
			Dir:              tempDir2,
			FileContents:     "content-2",
			TTLToDelete:      metav1.Duration{Duration: 2 * time.Minute},
			NumExpectedFiles: 2,
		}
		SetDefaultConfigs(pkgnfschecker.Configs{config2})

		retrieved2 := GetDefaultConfigs()
		assert.Len(t, retrieved2, 1)
		assert.Equal(t, config2.Dir, retrieved2[0].Dir)
		assert.Equal(t, config2.FileContents, retrieved2[0].FileContents)
		assert.Equal(t, config2.TTLToDelete.Duration, retrieved2[0].TTLToDelete.Duration)
		assert.Equal(t, config2.NumExpectedFiles, retrieved2[0].NumExpectedFiles)

		// Should not match the first config anymore
		assert.NotEqual(t, config1.Dir, retrieved2[0].Dir)
		assert.NotEqual(t, config1.FileContents, retrieved2[0].FileContents)
	})

	t.Run("concurrent access", func(t *testing.T) {
		tempDir := t.TempDir()

		config := pkgnfschecker.Config{
			Dir:              tempDir,
			FileContents:     "concurrent-test",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 10,
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
			assert.Equal(t, config.Dir, resultConfig.Dir)
			assert.Equal(t, config.FileContents, resultConfig.FileContents)
			assert.Equal(t, config.TTLToDelete.Duration, resultConfig.TTLToDelete.Duration)
			assert.Equal(t, config.NumExpectedFiles, resultConfig.NumExpectedFiles)
		}
	})

	t.Run("set multiple configs", func(t *testing.T) {
		tempDir1 := t.TempDir()
		tempDir2 := t.TempDir()

		config1 := pkgnfschecker.Config{
			Dir:              tempDir1,
			FileContents:     "content-1",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		config2 := pkgnfschecker.Config{
			Dir:              tempDir2,
			FileContents:     "content-2",
			TTLToDelete:      metav1.Duration{Duration: 2 * time.Minute},
			NumExpectedFiles: 2,
		}

		// Set multiple configs
		SetDefaultConfigs(pkgnfschecker.Configs{config1, config2})

		retrieved := GetDefaultConfigs()
		assert.Len(t, retrieved, 2)

		// Check first config
		assert.Equal(t, config1.Dir, retrieved[0].Dir)
		assert.Equal(t, config1.FileContents, retrieved[0].FileContents)
		assert.Equal(t, config1.TTLToDelete.Duration, retrieved[0].TTLToDelete.Duration)
		assert.Equal(t, config1.NumExpectedFiles, retrieved[0].NumExpectedFiles)

		// Check second config
		assert.Equal(t, config2.Dir, retrieved[1].Dir)
		assert.Equal(t, config2.FileContents, retrieved[1].FileContents)
		assert.Equal(t, config2.TTLToDelete.Duration, retrieved[1].TTLToDelete.Duration)
		assert.Equal(t, config2.NumExpectedFiles, retrieved[1].NumExpectedFiles)
	})
}
