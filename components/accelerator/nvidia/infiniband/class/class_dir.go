package class

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type classDirInterface interface {
	exists(path string) (bool, error)
	readFile(file string) (string, error)
	listDir(dir string) ([]fs.DirEntry, error)
}

var _ classDirInterface = &classDir{}

type classDir struct {
	rootDir string
	fs.FS
}

// e.g., default "/sys/class/infiniband"
func newClassDirInterface(rootDir string) (classDirInterface, error) {
	info, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("could not read %q: %w", rootDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("mount point %q is not a directory", rootDir)
	}
	return &classDir{
		rootDir: rootDir,
		FS:      os.DirFS(rootDir),
	}, nil
}

func (fs *classDir) exists(path string) (bool, error) {
	_, err := os.Stat(filepath.Join(fs.rootDir, path))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (fs *classDir) readFile(file string) (string, error) {
	b, err := fs.Open(file)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = b.Close()
	}()

	value, err := io.ReadAll(b)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(value)), nil
}

func (fs *classDir) listDir(dir string) ([]fs.DirEntry, error) {
	return os.ReadDir(filepath.Join(fs.rootDir, dir))
}
