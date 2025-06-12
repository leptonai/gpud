package nfschecker

import (
	"errors"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Config is a common configuration for all the NFS checker group
// members, which then translates into a single NFS checker.
type Config struct {
	// Dir is the directory where all the checkers in the group
	// write and read.
	Dir string `json:"dir"`

	// FileContents is the file contents to write the file with
	// and also the expected file contents to be read from other
	// files in the group directory. Meaning all other group members
	// write the same file contents to the directory.
	FileContents string `json:"file_contents"`

	// TTLToDelete is the duration that can elapse before files can be deleted
	// this is set to avoid counting the old files as valid data.
	TTLToDelete metav1.Duration `json:"ttl_to_delete"`

	// NumExpectedFiles is the count of files that are expected to be read
	// from the directory.
	NumExpectedFiles int `json:"num_expected_files,omitempty"`
}

// Configs is a list of GroupConfig.
type Configs []Config

// Validate validates the group configurations.
func (cfgs Configs) Validate() error {
	for _, cfg := range cfgs {
		if err := cfg.ValidateAndMkdir(); err != nil {
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
	ErrDirEmpty          = errors.New("directory is empty")
	ErrAbsDir            = errors.New("directory is not absolute")
	ErrDirNotExists      = errors.New("directory does not exist and cannot be created")
	ErrFileContentsEmpty = errors.New("file content is empty")
	ErrTTLZero           = errors.New("TTL is zero")
	ErrExpectedFilesZero = errors.New("expected files is zero")
)

// ValidateAndMkdir validates the configuration
// and creates the target directory if it does not exist.
func (c *Config) ValidateAndMkdir() error {
	if c.Dir == "" {
		return ErrDirEmpty
	}

	// e.g., ".gpud-nfs-checker" given as a relative path
	if !filepath.IsAbs(c.Dir) {
		return ErrAbsDir
	}

	if _, err := os.Stat(c.Dir); os.IsNotExist(err) {
		// e.g., "/data/.gpud-nfs-checker"
		// then we should mkdir ".gpud-nfs-checker" in "/data"
		if err := os.MkdirAll(c.Dir, 0755); err != nil {
			return ErrDirNotExists
		}
	} else if err != nil {
		return err
	}

	if c.FileContents == "" {
		return ErrFileContentsEmpty
	}
	if c.TTLToDelete.Duration == 0 {
		return ErrTTLZero
	}
	if c.NumExpectedFiles == 0 {
		return ErrExpectedFilesZero
	}

	return nil
}
