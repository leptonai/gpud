package nfschecker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemberConfig_Validate(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		config  MemberConfig
		wantErr error
	}{
		{
			name: "valid config",
			config: MemberConfig{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir",
					FileContents: "test-content",
				},
				ID: "test-id",
			},
			wantErr: nil,
		},
		{
			name: "empty volume path",
			config: MemberConfig{
				Config: Config{
					VolumePath:   "",
					DirName:      "test-dir",
					FileContents: "test-content",
				},
				ID: "test-id",
			},
			wantErr: ErrVolumePathEmpty,
		},
		{
			name: "relative directory path",
			config: MemberConfig{
				Config: Config{
					VolumePath:   "relative/path",
					DirName:      "test-dir",
					FileContents: "test-content",
				},
				ID: "test-id",
			},
			wantErr: ErrVolumePathNotAbs,
		},
		{
			name: "directory does not exist",
			config: MemberConfig{
				Config: Config{
					VolumePath:   "/non/existent/dir",
					DirName:      "test-dir",
					FileContents: "test-content",
				},
				ID: "test-id",
			},
			wantErr: ErrVolumePathNotExists,
		},
		{
			name: "empty ID",
			config: MemberConfig{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir",
					FileContents: "test-content",
				},
				ID: "",
			},
			wantErr: ErrIDEmpty,
		},
		{
			name: "empty file contents",
			config: MemberConfig{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir",
					FileContents: "",
				},
				ID: "test-id",
			},
			wantErr: ErrFileContentsEmpty,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate(context.Background())
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMemberConfigs_Validate(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("empty slice", func(t *testing.T) {
		var configs MemberConfigs
		err := configs.Validate(context.Background())
		assert.NoError(t, err)
	})

	t.Run("all valid configs", func(t *testing.T) {
		configs := MemberConfigs{
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir-1",
					FileContents: "content-1",
				},
				ID: "member-1",
			},
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir-2",
					FileContents: "content-2",
				},
				ID: "member-2",
			},
		}

		err := configs.Validate(context.Background())
		assert.NoError(t, err)
	})

	t.Run("one invalid config - empty ID", func(t *testing.T) {
		configs := MemberConfigs{
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir-1",
					FileContents: "content-1",
				},
				ID: "member-1",
			},
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir-2",
					FileContents: "content-2",
				},
				ID: "", // Invalid: empty ID
			},
		}

		err := configs.Validate(context.Background())
		assert.ErrorIs(t, err, ErrIDEmpty)
	})

	t.Run("one invalid config - empty directory", func(t *testing.T) {
		configs := MemberConfigs{
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir-1",
					FileContents: "content-1",
				},
				ID: "member-1",
			},
			{
				Config: Config{
					VolumePath:   "", // Invalid: empty directory
					DirName:      "test-dir-2",
					FileContents: "content-2",
				},
				ID: "member-2",
			},
		}

		err := configs.Validate(context.Background())
		assert.ErrorIs(t, err, ErrVolumePathEmpty)
	})

	t.Run("multiple invalid configs", func(t *testing.T) {
		configs := MemberConfigs{
			{
				Config: Config{
					VolumePath:   "", // Invalid: empty directory
					DirName:      "test-dir-1",
					FileContents: "content-1",
				},
				ID: "member-1",
			},
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir-2",
					FileContents: "content-2",
				},
				ID: "", // Invalid: empty ID
			},
		}

		err := configs.Validate(context.Background())
		// Should return the first error encountered
		assert.ErrorIs(t, err, ErrVolumePathEmpty)
	})

	t.Run("single config", func(t *testing.T) {
		configs := MemberConfigs{
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir",
					FileContents: "test-content",
				},
				ID: "single-member",
			},
		}

		err := configs.Validate(context.Background())
		assert.NoError(t, err)
	})

	t.Run("configs with same ID", func(t *testing.T) {
		configs := MemberConfigs{
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir-1",
					FileContents: "content-1",
				},
				ID: "same-id",
			},
			{
				Config: Config{
					VolumePath:   tempDir,
					DirName:      "test-dir-2",
					FileContents: "content-2",
				},
				ID: "same-id", // Same ID is allowed in validation
			},
		}

		err := configs.Validate(context.Background())
		assert.NoError(t, err)
	})
}
