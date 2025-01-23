package mock

import (
	"fmt"
	"os"
	"path"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	mockDir string
)

func GetMockDir() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if mockDir != "" {
		return mockDir, nil
	}
	dir := path.Join(os.TempDir(), fmt.Sprintf("gpud-mock-%d", time.Now().Unix()))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp dir %s: %v", dir, err)
	}
	if err := os.Setenv("PATH", fmt.Sprintf("%s:%s", dir, os.Getenv("PATH"))); err != nil {
		return "", fmt.Errorf("failed to add %s into PATH: %v", dir, err)
	}
	mockDir = dir
	return dir, nil
}
