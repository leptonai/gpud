// Package nfschecker checks the health of the NFS mount points.
package nfschecker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	pkgfilecleaner "github.com/leptonai/gpud/pkg/file/cleaner"
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

	// ReadIDs is the list of all IDs that are present in the directory.
	ReadIDs []string `json:"read_ids,omitempty"`

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
		cfg:                cfg,
		listFilesByPattern: filepath.Glob,
	}, nil
}

var _ Checker = &checker{}

type checker struct {
	cfg *MemberConfig

	listFilesByPattern func(pattern string) ([]string, error)
}

// Write writes a file to the directory with the ID as the file name.
func (c *checker) Write() error {
	// make sure the directory is writable
	// permission bit "0755" is used to allow the group to read the files
	// and the owner to read and write the files.
	if err := os.MkdirAll(c.cfg.VolumePath, 0755); err != nil {
		return err
	}

	dir := filepath.Join(c.cfg.VolumePath, c.cfg.DirName)
	file := filepath.Join(dir, c.cfg.ID)
	if err := os.WriteFile(file, []byte(c.cfg.FileContents), 0644); err != nil {
		return err
	}

	log.Logger.Infow("successfully wrote file", "file", file)
	return nil
}

// Check checks the directory and returns the result,
// based on the configuration.
func (c *checker) Check() CheckResult {
	// list all files under this directory
	dir := filepath.Join(c.cfg.VolumePath, c.cfg.DirName)
	pattern := filepath.Join(dir, "*")

	matches, err := c.listFilesByPattern(pattern)
	if err != nil {
		return CheckResult{
			Dir:     dir,
			Message: "failed to list files",
			Error:   err.Error(),
		}
	}

	// sort in order to make the output consistent
	// between subsequent checks
	sort.Strings(matches)

	result := CheckResult{
		Dir: dir,
	}
	for _, file := range matches {
		result.ReadIDs = append(result.ReadIDs, filepath.Base(file))

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
	}

	if len(result.ReadIDs) < c.cfg.NumExpectedFiles {
		result.Message = "failed"
		result.Error = fmt.Sprintf("expected %d files, but only %d files were read", c.cfg.NumExpectedFiles, len(result.ReadIDs))
		return result
	}

	result.Message = fmt.Sprintf("successfully checked directory %q with %d files", c.cfg.VolumePath, len(matches))
	return result
}

// Clean cleans up the files in the directory with the TTL.
func (c *checker) Clean() error {
	// list all files under this directory
	dir := filepath.Join(c.cfg.VolumePath, c.cfg.DirName)
	pattern := filepath.Join(dir, "*")

	// remove all files that are older than the TTL
	// in order to make sure the checker only checks
	// the latest file writes/reads from other members in the group
	now := time.Now().UTC()
	before := now.Add(-c.cfg.TTLToDelete.Duration)

	return pkgfilecleaner.Clean(pattern, before)
}
