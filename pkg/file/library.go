package file

import "errors"

var (
	ErrLibraryEmpty    = errors.New("library name is empty")
	ErrLibraryNotFound = errors.New("library not found")
)

func FindLibrary(name string) (string, error) {
	return findLibrary(name)
}
