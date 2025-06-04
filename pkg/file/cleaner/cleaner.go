// Package cleaner provides a cleaner for files.
package cleaner

import (
	"os"
	"path/filepath"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

// Clean remove all files that's created/updated before the given timestamp.
func Clean(pattern string, before time.Time) error {
	return clean(pattern, before, os.RemoveAll)
}

// clean remove all files that's created/updated before the given timestamp.
func clean(pattern string, before time.Time, removeFunc func(file string) error) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, file := range matches {
		if UpdatedAt(file).Before(before) {
			log.Logger.Infow("removing file", "file", file)
			if err := removeFunc(file); err != nil {
				return err
			}
		}
	}
	return nil
}

// UpdatedAt returns the creation or last modified time of the file.
// If the file does not exist or errors, it returns the zero time.
func UpdatedAt(file string) time.Time {
	s, err := os.Stat(file)
	if err != nil {
		return time.Time{}
	}
	return s.ModTime()
}
