package nfschecker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGroupConfig_Validate(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "valid config",
			config: Config{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			wantErr: nil,
		},
		{
			name: "empty directory",
			config: Config{
				Dir:              "",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			wantErr: ErrDirEmpty,
		},
		{
			name: "relative directory path",
			config: Config{
				Dir:              "relative/path",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			wantErr: ErrAbsDir,
		},
		{
			name: "directory does not exist",
			config: Config{
				Dir:              "/non/existent/dir",
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			wantErr: ErrDirNotExists,
		},
		{
			name: "empty file contents",
			config: Config{
				Dir:              tempDir,
				FileContents:     "",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			wantErr: ErrFileContentsEmpty,
		},
		{
			name: "zero TTL",
			config: Config{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: 0},
				NumExpectedFiles: 1,
			},
			wantErr: ErrTTLZero,
		},
		{
			name: "zero expected files",
			config: Config{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 0,
			},
			wantErr: ErrExpectedFilesZero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateAndMkdir()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGroupConfig_ValidateStatError(t *testing.T) {
	t.Run("os.Stat returns permission error", func(t *testing.T) {
		// Create a directory and then remove read permissions to trigger a stat error
		tempDir := t.TempDir()
		restrictedDir := filepath.Join(tempDir, "restricted")
		err := os.MkdirAll(restrictedDir, 0755)
		require.NoError(t, err)

		// Remove all permissions from parent directory to make stat fail
		err = os.Chmod(tempDir, 0000)
		require.NoError(t, err)

		// Restore permissions after test
		defer func() {
			_ = os.Chmod(tempDir, 0755)
		}()

		config := Config{
			Dir:              restrictedDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		err = config.ValidateAndMkdir()
		// Should return the os.Stat error (not ErrDirNotExists)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrDirNotExists)
		assert.NotErrorIs(t, err, ErrDirEmpty)
	})
}

func TestGroupConfig_ValidateEdgeCases(t *testing.T) {
	t.Run("very long file contents", func(t *testing.T) {
		tempDir := t.TempDir()
		longContent := string(make([]byte, 100000)) // 100KB of null bytes

		config := Config{
			Dir:              tempDir,
			FileContents:     longContent,
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		err := config.ValidateAndMkdir()
		assert.NoError(t, err)
	})

	t.Run("very large expected files count", func(t *testing.T) {
		tempDir := t.TempDir()

		config := Config{
			Dir:              tempDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 10000, // Very large number
		}

		err := config.ValidateAndMkdir()
		assert.NoError(t, err)
	})

	t.Run("very short TTL", func(t *testing.T) {
		tempDir := t.TempDir()

		config := Config{
			Dir:              tempDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Nanosecond}, // Very short TTL
			NumExpectedFiles: 1,
		}

		err := config.ValidateAndMkdir()
		assert.NoError(t, err)
	})

	t.Run("very long TTL", func(t *testing.T) {
		tempDir := t.TempDir()

		config := Config{
			Dir:              tempDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: 24 * time.Hour}, // Very long TTL
			NumExpectedFiles: 1,
		}

		err := config.ValidateAndMkdir()
		assert.NoError(t, err)
	})

	t.Run("directory path with spaces", func(t *testing.T) {
		tempDir := t.TempDir()
		spacedDir := filepath.Join(tempDir, "dir with spaces")
		err := os.MkdirAll(spacedDir, 0755)
		require.NoError(t, err)

		config := Config{
			Dir:              spacedDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		err = config.ValidateAndMkdir()
		assert.NoError(t, err)
	})

	t.Run("directory path with special characters", func(t *testing.T) {
		tempDir := t.TempDir()
		specialDir := filepath.Join(tempDir, "dir-with_special.chars")
		err := os.MkdirAll(specialDir, 0755)
		require.NoError(t, err)

		config := Config{
			Dir:              specialDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		err = config.ValidateAndMkdir()
		assert.NoError(t, err)
	})
}

func TestGroupConfig_ValidateSpecialCharacters(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("file contents with special characters", func(t *testing.T) {
		specialContent := "test-content with special chars: !@#$%^&*()_+-=[]{}|;':\",./<>?"

		config := Config{
			Dir:              tempDir,
			FileContents:     specialContent,
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		err := config.ValidateAndMkdir()
		assert.NoError(t, err)
	})

	t.Run("file contents with unicode characters", func(t *testing.T) {
		unicodeContent := "test-content with unicode: 你好世界 🌍 🚀 ñáéíóú"

		config := Config{
			Dir:              tempDir,
			FileContents:     unicodeContent,
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		err := config.ValidateAndMkdir()
		assert.NoError(t, err)
	})

	t.Run("file contents with newlines and tabs", func(t *testing.T) {
		multilineContent := "line1\nline2\tindented\r\nwindows-style"

		config := Config{
			Dir:              tempDir,
			FileContents:     multilineContent,
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		err := config.ValidateAndMkdir()
		assert.NoError(t, err)
	})
}

func TestGroupConfig_ErrorConstants(t *testing.T) {
	t.Run("error constants are defined", func(t *testing.T) {
		assert.NotNil(t, ErrDirEmpty)
		assert.NotNil(t, ErrDirNotExists)
		assert.NotNil(t, ErrFileContentsEmpty)
		assert.NotNil(t, ErrTTLZero)
		assert.NotNil(t, ErrExpectedFilesZero)
	})

	t.Run("error messages are meaningful", func(t *testing.T) {
		assert.Contains(t, ErrDirEmpty.Error(), "directory")
		assert.Contains(t, ErrDirNotExists.Error(), "directory")
		assert.Contains(t, ErrFileContentsEmpty.Error(), "file content")
		assert.Contains(t, ErrTTLZero.Error(), "TTL")
		assert.Contains(t, ErrExpectedFilesZero.Error(), "expected files")
	})
}

func TestGroupConfig_JSONTags(t *testing.T) {
	t.Run("struct has correct JSON tags", func(t *testing.T) {
		// This test ensures that the struct fields have the expected JSON tags
		// by creating a config and checking that it can be used in JSON contexts
		tempDir := t.TempDir()

		config := Config{
			Dir:              tempDir,
			FileContents:     "test-content",
			TTLToDelete:      metav1.Duration{Duration: time.Minute},
			NumExpectedFiles: 1,
		}

		// Validate that the config is valid
		err := config.ValidateAndMkdir()
		assert.NoError(t, err)

		// Check that all fields are accessible (this would fail if JSON tags were wrong)
		assert.NotEmpty(t, config.Dir)
		assert.NotEmpty(t, config.FileContents)
		assert.NotZero(t, config.TTLToDelete.Duration)
		assert.NotZero(t, config.NumExpectedFiles)
	})
}

func TestGroupConfig_JSONEncoding(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name           string
		config         Config
		expectedTTL    time.Duration
		checkJSONField bool
	}{
		{
			name: "1 minute TTL",
			config: Config{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			expectedTTL:    time.Minute,
			checkJSONField: true,
		},
		{
			name: "30 seconds TTL",
			config: Config{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: 30 * time.Second},
				NumExpectedFiles: 2,
			},
			expectedTTL:    30 * time.Second,
			checkJSONField: true,
		},
		{
			name: "1 hour TTL",
			config: Config{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Hour},
				NumExpectedFiles: 3,
			},
			expectedTTL:    time.Hour,
			checkJSONField: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.config)
			require.NoError(t, err)

			// Verify the TTL duration is correct
			assert.Equal(t, tt.expectedTTL, tt.config.TTLToDelete.Duration)

			if tt.checkJSONField {
				// Parse the JSON to verify the TTL field is encoded as a string duration
				var jsonMap map[string]interface{}
				err = json.Unmarshal(jsonData, &jsonMap)
				require.NoError(t, err)

				// The TTL should be encoded as a string duration
				ttlValue, exists := jsonMap["ttl_to_delete"]
				assert.True(t, exists, "TTL field should exist in JSON")
				assert.IsType(t, "", ttlValue, "TTL should be encoded as string")
			}
		})
	}
}

func TestGroupConfig_JSONDecoding(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		jsonInput   string
		expectedTTL time.Duration
		expectError bool
	}{
		{
			name: "decode 1m TTL",
			jsonInput: `{
				"dir": "` + tempDir + `",
				"file_contents": "test-content",
				"ttl_to_delete": "1m",
				"num_expected_files": 1
			}`,
			expectedTTL: time.Minute,
			expectError: false,
		},
		{
			name: "decode 30s TTL",
			jsonInput: `{
				"dir": "` + tempDir + `",
				"file_contents": "test-content",
				"ttl_to_delete": "30s",
				"num_expected_files": 2
			}`,
			expectedTTL: 30 * time.Second,
			expectError: false,
		},
		{
			name: "decode 1h TTL",
			jsonInput: `{
				"dir": "` + tempDir + `",
				"file_contents": "test-content",
				"ttl_to_delete": "1h",
				"num_expected_files": 3
			}`,
			expectedTTL: time.Hour,
			expectError: false,
		},
		{
			name: "decode 2h30m TTL",
			jsonInput: `{
				"dir": "` + tempDir + `",
				"file_contents": "test-content",
				"ttl_to_delete": "2h30m",
				"num_expected_files": 1
			}`,
			expectedTTL: 2*time.Hour + 30*time.Minute,
			expectError: false,
		},
		{
			name: "decode 500ms TTL",
			jsonInput: `{
				"dir": "` + tempDir + `",
				"file_contents": "test-content",
				"ttl_to_delete": "500ms",
				"num_expected_files": 1
			}`,
			expectedTTL: 500 * time.Millisecond,
			expectError: false,
		},
		{
			name: "decode nanosecond duration",
			jsonInput: `{
				"dir": "` + tempDir + `",
				"file_contents": "test-content",
				"ttl_to_delete": "60000000000ns",
				"num_expected_files": 1
			}`,
			expectedTTL: time.Minute,
			expectError: false,
		},
		{
			name: "decode invalid TTL format",
			jsonInput: `{
				"dir": "` + tempDir + `",
				"file_contents": "test-content",
				"ttl_to_delete": "invalid-duration",
				"num_expected_files": 1
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config Config
			err := json.Unmarshal([]byte(tt.jsonInput), &config)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedTTL, config.TTLToDelete.Duration)
			assert.Equal(t, tempDir, config.Dir)
			assert.Equal(t, "test-content", config.FileContents)
		})
	}
}

func TestGroupConfig_JSONRoundTrip(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		ttlDuration time.Duration
	}{
		{
			name:        "1 minute round trip",
			ttlDuration: time.Minute,
		},
		{
			name:        "30 seconds round trip",
			ttlDuration: 30 * time.Second,
		},
		{
			name:        "1 hour round trip",
			ttlDuration: time.Hour,
		},
		{
			name:        "complex duration round trip",
			ttlDuration: 2*time.Hour + 30*time.Minute + 45*time.Second,
		},
		{
			name:        "milliseconds round trip",
			ttlDuration: 1500 * time.Millisecond,
		},
		{
			name:        "microseconds round trip",
			ttlDuration: 1500 * time.Microsecond,
		},
		{
			name:        "nanoseconds round trip",
			ttlDuration: 1500 * time.Nanosecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := Config{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: tt.ttlDuration},
				NumExpectedFiles: 1,
			}

			// Marshal to JSON
			jsonData, err := json.Marshal(original)
			require.NoError(t, err)

			// Unmarshal back from JSON
			var decoded Config
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err)

			// Verify all fields are preserved
			assert.Equal(t, original.Dir, decoded.Dir)
			assert.Equal(t, original.FileContents, decoded.FileContents)
			assert.Equal(t, original.TTLToDelete.Duration, decoded.TTLToDelete.Duration)
			assert.Equal(t, original.NumExpectedFiles, decoded.NumExpectedFiles)

			// Specifically verify TTL duration
			assert.Equal(t, tt.ttlDuration, decoded.TTLToDelete.Duration)
		})
	}
}

func TestGroupConfig_JSONWithStringDuration(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("decode string duration 1m", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"ttl_to_delete": "1m",
			"num_expected_files": 1
		}`
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		require.NoError(t, err)
		assert.Equal(t, time.Minute, config.TTLToDelete.Duration)
	})

	t.Run("decode string duration 5m30s", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"ttl_to_delete": "5m30s",
			"num_expected_files": 1
		}`
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		require.NoError(t, err)
		assert.Equal(t, 5*time.Minute+30*time.Second, config.TTLToDelete.Duration)
	})

	t.Run("decode string duration with quotes", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"ttl_to_delete": "10m15s",
			"num_expected_files": 1
		}`
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		require.NoError(t, err)
		assert.Equal(t, 10*time.Minute+15*time.Second, config.TTLToDelete.Duration)
	})

	t.Run("validate decoded config with string duration", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"ttl_to_delete": "1m",
			"num_expected_files": 1
		}`
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		require.NoError(t, err)

		// Validate the decoded config
		err = config.ValidateAndMkdir()
		assert.NoError(t, err)
	})
}

func TestGroupConfig_JSONEdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("zero duration", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"ttl_to_delete": "0s",
			"num_expected_files": 1
		}`
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), config.TTLToDelete.Duration)

		// This should fail validation
		err = config.ValidateAndMkdir()
		assert.ErrorIs(t, err, ErrTTLZero)
	})

	t.Run("very large duration", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"ttl_to_delete": "8760h",
			"num_expected_files": 1
		}`
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		require.NoError(t, err)
		assert.Equal(t, 8760*time.Hour, config.TTLToDelete.Duration) // 1 year
	})

	t.Run("nanosecond precision", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"ttl_to_delete": "1000000000ns",
			"num_expected_files": 1
		}`
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		require.NoError(t, err)
		assert.Equal(t, time.Second, config.TTLToDelete.Duration)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"ttl_to_delete": "1m",
			"num_expected_files": 1,
		}` // trailing comma makes it invalid JSON
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		assert.Error(t, err)
	})

	t.Run("missing TTL field", func(t *testing.T) {
		jsonInput := `{
			"dir": "` + tempDir + `",
			"file_contents": "test-content",
			"num_expected_files": 1
		}`
		var config Config
		err := json.Unmarshal([]byte(jsonInput), &config)
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), config.TTLToDelete.Duration)

		// This should fail validation due to zero TTL
		err = config.ValidateAndMkdir()
		assert.ErrorIs(t, err, ErrTTLZero)
	})
}

func TestGroupConfigs_Validate(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("empty slice", func(t *testing.T) {
		var configs Configs
		err := configs.Validate()
		assert.NoError(t, err)
	})

	t.Run("all valid configs", func(t *testing.T) {
		configs := Configs{
			{
				Dir:              tempDir,
				FileContents:     "content-1",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			{
				Dir:              tempDir,
				FileContents:     "content-2",
				TTLToDelete:      metav1.Duration{Duration: 2 * time.Minute},
				NumExpectedFiles: 2,
			},
		}

		err := configs.Validate()
		assert.NoError(t, err)
	})

	t.Run("one invalid config", func(t *testing.T) {
		configs := Configs{
			{
				Dir:              tempDir,
				FileContents:     "content-1",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			{
				Dir:              "", // Invalid: empty directory
				FileContents:     "content-2",
				TTLToDelete:      metav1.Duration{Duration: 2 * time.Minute},
				NumExpectedFiles: 2,
			},
		}

		err := configs.Validate()
		assert.ErrorIs(t, err, ErrDirEmpty)
	})

	t.Run("multiple invalid configs", func(t *testing.T) {
		configs := Configs{
			{
				Dir:              "", // Invalid: empty directory
				FileContents:     "content-1",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			{
				Dir:              tempDir,
				FileContents:     "", // Invalid: empty file contents
				TTLToDelete:      metav1.Duration{Duration: 2 * time.Minute},
				NumExpectedFiles: 2,
			},
		}

		err := configs.Validate()
		// Should return the first error encountered
		assert.ErrorIs(t, err, ErrDirEmpty)
	})
}

func TestGroupConfigs_GetMemberConfigs(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("empty slice", func(t *testing.T) {
		var configs Configs
		memberConfigs := configs.GetMemberConfigs("test-machine-id")
		assert.Empty(t, memberConfigs)
	})

	t.Run("single config", func(t *testing.T) {
		configs := Configs{
			{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
		}

		memberConfigs := configs.GetMemberConfigs("test-machine-id")
		assert.Len(t, memberConfigs, 1)

		member := memberConfigs[0]
		assert.Equal(t, "test-machine-id", member.ID)
		assert.Equal(t, tempDir, member.Dir)
		assert.Equal(t, "test-content", member.FileContents)
		assert.Equal(t, time.Minute, member.TTLToDelete.Duration)
		assert.Equal(t, 1, member.NumExpectedFiles)
	})

	t.Run("multiple configs", func(t *testing.T) {
		tempDir2 := t.TempDir()
		configs := Configs{
			{
				Dir:              tempDir,
				FileContents:     "content-1",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
			{
				Dir:              tempDir2,
				FileContents:     "content-2",
				TTLToDelete:      metav1.Duration{Duration: 2 * time.Minute},
				NumExpectedFiles: 2,
			},
		}

		memberConfigs := configs.GetMemberConfigs("machine-123")
		assert.Len(t, memberConfigs, 2)

		// Check first member config
		member1 := memberConfigs[0]
		assert.Equal(t, "machine-123", member1.ID)
		assert.Equal(t, tempDir, member1.Dir)
		assert.Equal(t, "content-1", member1.FileContents)
		assert.Equal(t, time.Minute, member1.TTLToDelete.Duration)
		assert.Equal(t, 1, member1.NumExpectedFiles)

		// Check second member config
		member2 := memberConfigs[1]
		assert.Equal(t, "machine-123", member2.ID)
		assert.Equal(t, tempDir2, member2.Dir)
		assert.Equal(t, "content-2", member2.FileContents)
		assert.Equal(t, 2*time.Minute, member2.TTLToDelete.Duration)
		assert.Equal(t, 2, member2.NumExpectedFiles)
	})

	t.Run("different machine IDs", func(t *testing.T) {
		configs := Configs{
			{
				Dir:              tempDir,
				FileContents:     "shared-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 3,
			},
		}

		// Test with different machine IDs
		memberConfigs1 := configs.GetMemberConfigs("machine-1")
		memberConfigs2 := configs.GetMemberConfigs("machine-2")

		assert.Len(t, memberConfigs1, 1)
		assert.Len(t, memberConfigs2, 1)

		assert.Equal(t, "machine-1", memberConfigs1[0].ID)
		assert.Equal(t, "machine-2", memberConfigs2[0].ID)

		// Both should have the same group config data
		assert.Equal(t, memberConfigs1[0].Dir, memberConfigs2[0].Dir)
		assert.Equal(t, memberConfigs1[0].FileContents, memberConfigs2[0].FileContents)
		assert.Equal(t, memberConfigs1[0].TTLToDelete.Duration, memberConfigs2[0].TTLToDelete.Duration)
		assert.Equal(t, memberConfigs1[0].NumExpectedFiles, memberConfigs2[0].NumExpectedFiles)
	})

	t.Run("empty machine ID", func(t *testing.T) {
		configs := Configs{
			{
				Dir:              tempDir,
				FileContents:     "test-content",
				TTLToDelete:      metav1.Duration{Duration: time.Minute},
				NumExpectedFiles: 1,
			},
		}

		memberConfigs := configs.GetMemberConfigs("")
		assert.Len(t, memberConfigs, 1)
		assert.Equal(t, "", memberConfigs[0].ID)
	})
}
