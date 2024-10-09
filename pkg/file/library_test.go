package file

import (
	"testing"
)

func TestFindLibrary(t *testing.T) {
	_, err := FindLibrary("")
	if err != ErrLibraryEmpty {
		t.Errorf("FindLibrary() error = %v, want %v", err, ErrLibraryEmpty)
	}
}
