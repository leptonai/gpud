package file

import (
	"errors"
	"os"
	"path/filepath"
)

var (
	ErrLibraryEmpty    = errors.New("library name is empty")
	ErrLibraryNotFound = errors.New("library not found")
	ErrEmptySearchDir  = errors.New("empty search dir")
)

func FindLibrary(name string, opts ...OpOption) (string, error) {
	options := &Op{}
	if err := options.applyOpts(opts); err != nil {
		return "", err
	}

	return findLibrary(options.searchDirs, name, options.alternativeLibraryNames)
}

type Op struct {
	alternativeLibraryNames map[string]any
	searchDirs              map[string]any
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	if len(op.searchDirs) == 0 {
		return ErrEmptySearchDir
	}
	return nil
}

func WithAlternativeLibraryName(name string) OpOption {
	return func(op *Op) {
		if op.alternativeLibraryNames == nil {
			op.alternativeLibraryNames = make(map[string]any)
		}
		op.alternativeLibraryNames[name] = struct{}{}
	}
}

func WithSearchDirs(paths ...string) OpOption {
	return func(op *Op) {
		if op.searchDirs == nil {
			op.searchDirs = make(map[string]any)
		}
		for _, path := range paths {
			op.searchDirs[path] = struct{}{}
		}
	}
}

// Returns the resolved path of the library if found.
// Returns an empty string and no error if not found.
func findLibrary(searchDirs map[string]any, name string, alternativeLibraryNames map[string]any) (string, error) {
	if name == "" {
		return "", ErrLibraryEmpty
	}

	// first try the exact name
	names := []string{name}

	if len(alternativeLibraryNames) > 0 {
		for altName := range alternativeLibraryNames {
			names = append(names, altName)
		}
	}

	for dir := range searchDirs {
		exists, err := directoryExists(dir)
		if err != nil {
			return "", err
		}
		if !exists {
			continue
		}

		libPath, err := locateLib(dir, names)

		// retry in next dir
		if errors.Is(err, ErrLibraryNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}

		return libPath, nil
	}

	return "", ErrLibraryNotFound
}

func locateLib(dir string, names []string) (string, error) {
	for _, name := range names {
		libPath := filepath.Join(dir, name)

		exists, err := fileExists(libPath)
		if err != nil {
			return "", err
		}

		if !exists {
			continue
		}

		p, err := filepath.EvalSymlinks(libPath)
		if err == nil {
			return p, nil
		}
	}
	return "", ErrLibraryNotFound
}

// returns true if the directory exists
func directoryExists(dir string) (bool, error) {
	if dir == "" {
		return false, nil
	}
	fileInfo, err := os.Stat(dir)
	if err == nil {
		return fileInfo.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// returns true if the file or directory exists
func fileExists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
