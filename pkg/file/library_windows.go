//go:build windows
// +build windows

package file

func findLibrary(name string) (string, error) {
	if name == "" {
		return "", ErrLibraryEmpty
	}
	return "", nil
}
