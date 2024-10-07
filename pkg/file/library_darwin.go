//go:build darwin
// +build darwin

package file

func findLibrary(_ map[string]any, name string) (string, error) {
	if name == "" {
		return "", ErrLibraryEmpty
	}
	return "", nil
}
