// Package file implements file utils.
package file

import (
	"fmt"
	"os"
	"os/exec"
)

func LocateExecutable(bin string) (string, error) {
	execPath, err := exec.LookPath(bin)
	if err == nil {
		return execPath, CheckExecutable(execPath)
	}
	return "", fmt.Errorf("executable %q not found in PATH: %w", bin, err)
}

func CheckExecutable(file string) error {
	s, err := os.Stat(file)
	if err != nil {
		return err
	}

	if s.IsDir() {
		return fmt.Errorf("%q is a directory", file)
	}

	if s.Mode()&0111 == 0 {
		return fmt.Errorf("%q is not executable", file)
	}

	return nil
}
