// Package nfschecker checks the health of the NFS mount points.
package nfschecker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leptonai/gpud/pkg/log"
)

// Checker checks the health of the NFS mount points
// by writing a file and reading other files in the same directory.
type Checker interface {
	// Write writes a file to the directory with the ID as the file name.
	Write() error
	// Check checks the directory and returns the result,
	// based on the configuration.
	Check() CheckResult
	// Clean cleans up the files in the directory with the TTL.
	Clean() error
}

// CheckResult is the result of the check.
type CheckResult struct {
	// Dir is the directory that is checked.
	Dir string `json:"dir"`

	// Message is the message of the check result.
	Message string `json:"message"`

	// Error contains any system error during checks
	// or validation errors.
	// Set to an empty string, if there was no error, and
	// validation succeeded.
	Error string `json:"error,omitempty"`
}

// NewChecker creates a new checker with the given configuration.
func NewChecker(cfg *MemberConfig) (Checker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &checker{
		cfg: cfg,
	}, nil
}

var _ Checker = &checker{}

type checker struct {
	cfg *MemberConfig
}

// Write writes a file to the directory with the ID as the file name.
func (c *checker) Write() error {
	// make sure the directory is writable
	// permission bit "0755" is used to allow the group to read the files
	// and the owner to read and write the files.
	if err := os.MkdirAll(c.cfg.VolumePath, 0755); err != nil {
		return err
	}

	file := c.cfg.fileSelf()
	if err := os.WriteFile(file, []byte(c.cfg.FileContents), 0644); err != nil {
		return err
	}

	log.Logger.Infow("successfully wrote file", "file", file)
	return nil
}

// Check checks the directory and returns the result,
// based on the configuration.
func (c *checker) Check() CheckResult {
	dir := filepath.Join(c.cfg.VolumePath, c.cfg.DirName)
	file := c.cfg.fileSelf()

	result := CheckResult{
		Dir: dir,
	}

	contents, err := os.ReadFile(file)
	if err != nil {
		result.Message = "failed"
		result.Error = fmt.Sprintf("failed to read file %s: %s", file, err)
		return result
	}

	if string(contents) != c.cfg.FileContents {
		result.Message = "failed"
		result.Error = fmt.Sprintf("file %q has unexpected contents", file)
		return result
	}

	result.Message = fmt.Sprintf("correctly read/wrote on %q", c.cfg.VolumePath)
	return result
}

// Clean cleans up the file that is written by the checker.
func (c *checker) Clean() error {
	file := c.cfg.fileSelf()
	return os.RemoveAll(file)
}
