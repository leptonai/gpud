package lspci

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	e2emock "github.com/leptonai/gpud/e2e/mock"
)

var (
	mu       sync.Mutex
	mockFile string
)

func Mock(output string) error {
	mu.Lock()
	defer mu.Unlock()

	dir, err := e2emock.GetMockDir()
	if err != nil {
		return err
	}
	mockFile = filepath.Join(dir, "lspci")
	content := fmt.Sprintf(scriptTemplate, output)
	return os.WriteFile(mockFile, []byte(content), 0755)
}

var scriptTemplate = `#!/bin/bash

while [[ $# -gt 0 ]]; do
  case "$1" in
    *)
      shift
      ;;
  esac
done

echo "%s"
`
