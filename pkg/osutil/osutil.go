// Package osutil provides utilities for the operating system.
package osutil

import (
	"errors"
	"os"
)

func RequireRoot() error {
	if os.Geteuid() == 0 {
		return nil
	}
	return errors.New("this command needs to be run as root")
}
