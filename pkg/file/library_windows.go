//go:build windows
// +build windows

package file

func findLibrary(_ map[string]any, name string) (string, error) {
	if name == "" {
		return "", ErrLibraryEmpty
	}
	return "", nil
}
