package nfschecker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	pkgfile "github.com/leptonai/gpud/pkg/file"
)

// Config is a common configuration for all the NFS checker group
// members, which then translates into a single NFS checker.
type Config struct {
	// VolumePath is the volume path to test its NFS mounts.
	// Must be writable by the NFS checker group members.
	// And writes are saved in the [Config.VolumePath] under [Config.DirName].
	// This path must be an absolute path.
	VolumePath string `json:"volume_path"`

	// DirName is the directory name under [Config.VolumePath]
	// to write and read the files.
	// e.g., ".gpud-nfs-checker"
	DirName string `json:"dir_name"`

	// FileContents is the file contents to write the file with
	// and also the expected file contents to be read from other
	// files in the group directory. Meaning all other group members
	// write the same file contents to the directory.
	FileContents string `json:"file_contents"`
}

// Configs is a list of GroupConfig.
type Configs []Config

// Validate validates the group configurations.
func (cfgs Configs) Validate(ctx context.Context) error {
	for _, cfg := range cfgs {
		if err := cfg.ValidateAndMkdir(ctx); err != nil {
			return err
		}
	}
	return nil
}

// GetMemberConfigs converts GroupConfigs to MemberConfigs by adding the provided machine ID.
func (cfgs Configs) GetMemberConfigs(machineID string) MemberConfigs {
	memberConfigs := make(MemberConfigs, 0, len(cfgs))
	for _, cfg := range cfgs {
		memberConfig := MemberConfig{
			Config: cfg,
			ID:     machineID,
		}
		memberConfigs = append(memberConfigs, memberConfig)
	}
	return memberConfigs
}

var (
	ErrVolumePathEmpty     = errors.New("volume path is empty")
	ErrVolumePathNotAbs    = errors.New("volume path is not absolute")
	ErrVolumePathNotExists = errors.New("volume path does not exist")
	ErrFileContentsEmpty   = errors.New("file content is empty")
)

// ValidateAndMkdir validates the configuration
// and creates the target directory if it does not exist.
func (c *Config) ValidateAndMkdir(ctx context.Context) error {
	if c.VolumePath == "" {
		return ErrVolumePathEmpty
	}

	// e.g., ".gpud-nfs-checker" given as a relative path
	if !filepath.IsAbs(c.VolumePath) {
		return ErrVolumePathNotAbs
	}

	if _, err := pkgfile.StatWithTimeout(ctx, c.VolumePath); os.IsNotExist(err) {
		return fmt.Errorf("%q %w", c.VolumePath, ErrVolumePathNotExists)
	} else if err != nil {
		return err
	}

	dir := filepath.Join(c.VolumePath, c.DirName)
	if _, err := pkgfile.StatWithTimeout(ctx, dir); os.IsNotExist(err) {
		if err := pkgfile.MkdirAllWithTimeout(ctx, dir, 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if c.FileContents == "" {
		return ErrFileContentsEmpty
	}

	return nil
}
