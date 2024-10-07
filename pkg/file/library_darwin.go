//go:build darwin
// +build darwin

package file

func findLibrary(name string) (string, error) {
	if name == "" {
		return "", ErrLibraryEmpty
	}
	return "", nil
}
