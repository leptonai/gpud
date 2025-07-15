package nfschecker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				VolumePath:   tempDir,
				DirName:      "test-dir",
				FileContents: "test-content",
			},
			wantErr: nil,
		},
		{
			name: "empty volume path",
			config: Config{
				VolumePath:   "",
				DirName:      "test-dir",
				FileContents: "test-content",
			},
			wantErr: ErrVolumePathEmpty,
		},
		{
			name: "relative directory path",
			config: Config{
				VolumePath:   "relative/path",
				DirName:      "test-dir",
				FileContents: "test-content",
			},
			wantErr: ErrVolumePathNotAbs,
		},
		{
			name: "directory does not exist",
			config: Config{
				VolumePath:   "/non/existent/dir",
				DirName:      "test-dir",
				FileContents: "test-content",
			},
			wantErr: ErrVolumePathNotExists,
		},
		{
			name: "empty file contents",
			config: Config{
				VolumePath:   tempDir,
				DirName:      "test-dir",
				FileContents: "",
			},
			wantErr: ErrFileContentsEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.config.ValidateAndMkdir(ctx)
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
			VolumePath:   restrictedDir,
			DirName:      "test-dir",
			FileContents: "test-content",
		}

		ctx := context.Background()
		err = config.ValidateAndMkdir(ctx)
		// Should return the os.Stat error (not ErrDirNotExists)
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrVolumePathNotExists)
		assert.NotErrorIs(t, err, ErrVolumePathEmpty)
	})
}

func TestGroupConfig_ValidateEdgeCases(t *testing.T) {
	t.Run("very long file contents", func(t *testing.T) {
		tempDir := t.TempDir()
		longContent := string(make([]byte, 100000)) // 100KB of null bytes

		config := Config{
			VolumePath:   tempDir,
			DirName:      "test-dir",
			FileContents: longContent,
		}

		ctx := context.Background()
		err := config.ValidateAndMkdir(ctx)
		assert.NoError(t, err)
	})

	t.Run("directory path with spaces", func(t *testing.T) {
		tempDir := t.TempDir()
		spacedDir := filepath.Join(tempDir, "dir with spaces")
		err := os.MkdirAll(spacedDir, 0755)
		require.NoError(t, err)

		config := Config{
			VolumePath:   spacedDir,
			DirName:      "test-dir",
			FileContents: "test-content",
		}

		ctx := context.Background()
		err = config.ValidateAndMkdir(ctx)
		assert.NoError(t, err)
	})

	t.Run("directory path with special characters", func(t *testing.T) {
		tempDir := t.TempDir()
		specialDir := filepath.Join(tempDir, "dir-with_special.chars")
		err := os.MkdirAll(specialDir, 0755)
		require.NoError(t, err)

		config := Config{
			VolumePath:   specialDir,
			DirName:      "test-dir",
			FileContents: "test-content",
		}

		ctx := context.Background()
		err = config.ValidateAndMkdir(ctx)
		assert.NoError(t, err)
	})
}

func TestGroupConfig_ValidateSpecialCharacters(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("file contents with special characters", func(t *testing.T) {
		specialContent := "test-content with special chars: !@#$%^&*()_+-=[]{}|;':\",./<>?"

		config := Config{
			VolumePath:   tempDir,
			DirName:      "test-dir",
			FileContents: specialContent,
		}

		ctx := context.Background()
		err := config.ValidateAndMkdir(ctx)
		assert.NoError(t, err)
	})

	t.Run("file contents with unicode characters", func(t *testing.T) {
		unicodeContent := "test-content with unicode: 你好世界 🌍 🚀 ñáéíóú"

		config := Config{
			VolumePath:   tempDir,
			DirName:      "test-dir",
			FileContents: unicodeContent,
		}

		ctx := context.Background()
		err := config.ValidateAndMkdir(ctx)
		assert.NoError(t, err)
	})

	t.Run("file contents with newlines and tabs", func(t *testing.T) {
		multilineContent := "line1\nline2\tindented\r\nwindows-style"

		config := Config{
			VolumePath:   tempDir,
			DirName:      "test-dir",
			FileContents: multilineContent,
		}

		ctx := context.Background()
		err := config.ValidateAndMkdir(ctx)
		assert.NoError(t, err)
	})
}

func TestGroupConfig_ErrorConstants(t *testing.T) {
	t.Run("error constants are defined", func(t *testing.T) {
		assert.NotNil(t, ErrVolumePathEmpty)
		assert.NotNil(t, ErrVolumePathNotAbs)
		assert.NotNil(t, ErrVolumePathNotExists)
		assert.NotNil(t, ErrFileContentsEmpty)
	})

	t.Run("error messages are meaningful", func(t *testing.T) {
		assert.Contains(t, ErrVolumePathEmpty.Error(), "volume path")
		assert.Contains(t, ErrVolumePathNotAbs.Error(), "volume path")
		assert.Contains(t, ErrVolumePathNotExists.Error(), "volume path")
		assert.Contains(t, ErrFileContentsEmpty.Error(), "file content")
	})
}

func TestGroupConfigs_Validate(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("empty slice", func(t *testing.T) {
		var configs Configs
		ctx := context.Background()
		err := configs.Validate(ctx)
		assert.NoError(t, err)
	})

	t.Run("all valid configs", func(t *testing.T) {
		configs := Configs{
			{
				VolumePath:   tempDir,
				DirName:      "test-dir-1",
				FileContents: "content-1",
			},
			{
				VolumePath:   tempDir,
				DirName:      "test-dir-2",
				FileContents: "content-2",
			},
		}

		ctx := context.Background()
		err := configs.Validate(ctx)
		assert.NoError(t, err)
	})

	t.Run("one invalid config", func(t *testing.T) {
		configs := Configs{
			{
				VolumePath:   tempDir,
				DirName:      "test-dir-1",
				FileContents: "content-1",
			},
			{
				VolumePath:   "", // Invalid: empty directory
				DirName:      "test-dir-2",
				FileContents: "content-2",
			},
		}

		ctx := context.Background()
		err := configs.Validate(ctx)
		assert.ErrorIs(t, err, ErrVolumePathEmpty)
	})

	t.Run("multiple invalid configs", func(t *testing.T) {
		configs := Configs{
			{
				VolumePath:   "", // Invalid: empty directory
				DirName:      "test-dir-1",
				FileContents: "content-1",
			},
			{
				VolumePath:   tempDir,
				DirName:      "test-dir-2",
				FileContents: "", // Invalid: empty file contents
			},
		}

		ctx := context.Background()
		err := configs.Validate(ctx)
		// Should return the first error encountered
		assert.ErrorIs(t, err, ErrVolumePathEmpty)
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
				VolumePath:   tempDir,
				DirName:      "test-dir",
				FileContents: "test-content",
			},
		}

		memberConfigs := configs.GetMemberConfigs("test-machine-id")
		assert.Len(t, memberConfigs, 1)

		member := memberConfigs[0]
		assert.Equal(t, "test-machine-id", member.ID)
		assert.Equal(t, tempDir, member.VolumePath)
		assert.Equal(t, "test-content", member.FileContents)
	})

	t.Run("multiple configs", func(t *testing.T) {
		tempDir2 := t.TempDir()
		configs := Configs{
			{
				VolumePath:   tempDir,
				DirName:      "test-dir-1",
				FileContents: "content-1",
			},
			{
				VolumePath:   tempDir2,
				DirName:      "test-dir-2",
				FileContents: "content-2",
			},
		}

		memberConfigs := configs.GetMemberConfigs("machine-123")
		assert.Len(t, memberConfigs, 2)

		// Check first member config
		member1 := memberConfigs[0]
		assert.Equal(t, "machine-123", member1.ID)
		assert.Equal(t, tempDir, member1.VolumePath)
		assert.Equal(t, "content-1", member1.FileContents)

		// Check second member config
		member2 := memberConfigs[1]
		assert.Equal(t, "machine-123", member2.ID)
		assert.Equal(t, tempDir2, member2.VolumePath)
		assert.Equal(t, "content-2", member2.FileContents)
	})

	t.Run("different machine IDs", func(t *testing.T) {
		configs := Configs{
			{
				VolumePath:   tempDir,
				DirName:      "test-dir",
				FileContents: "shared-content",
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
		assert.Equal(t, memberConfigs1[0].VolumePath, memberConfigs2[0].VolumePath)
		assert.Equal(t, memberConfigs1[0].FileContents, memberConfigs2[0].FileContents)
	})

	t.Run("empty machine ID", func(t *testing.T) {
		configs := Configs{
			{
				VolumePath:   tempDir,
				DirName:      "test-dir",
				FileContents: "test-content",
			},
		}

		memberConfigs := configs.GetMemberConfigs("")
		assert.Len(t, memberConfigs, 1)
		assert.Equal(t, "", memberConfigs[0].ID)
	})
}
