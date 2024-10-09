//go:build windows
// +build windows

package file

var defaultLibSearchDirs = map[string]any{}

func findLibrary(_ map[string]any, name string) (string, error) {
	if name == "" {
		return "", ErrLibraryEmpty
	}
	return "", nil
}
