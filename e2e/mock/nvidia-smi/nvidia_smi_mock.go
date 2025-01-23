package nvidia_smi

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

func Mock(smiOutput, queryOutput string) error {
	mu.Lock()
	defer mu.Unlock()

	dir, err := e2emock.GetMockDir()
	if err != nil {
		return err
	}
	mockFile = filepath.Join(dir, "nvidia-smi")
	content := fmt.Sprintf(scriptTemplate, queryOutput, smiOutput)
	return os.WriteFile(mockFile, []byte(content), 0755)
}

var scriptTemplate = `#!/bin/bash

query_mode=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    -q|--query)
      query_mode=true
      shift
      ;;
    *)
      shift
      ;;
  esac
done

if $query_mode; then
  echo "%s"
else
  echo "%s"
fi
`
