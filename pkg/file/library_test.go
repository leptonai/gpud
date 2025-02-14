package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindLibrary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "library_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files
	testLib := filepath.Join(tmpDir, "test.so")
	altLib := filepath.Join(tmpDir, "test.so.1")
	_, err = os.Create(testLib)
	assert.NoError(t, err)
	_, err = os.Create(altLib)
	assert.NoError(t, err)

	// Create a symlink that points to a non-existent file
	brokenSymlink := filepath.Join(tmpDir, "broken.so")
	err = os.Symlink(filepath.Join(tmpDir, "nonexistent.so"), brokenSymlink)
	assert.NoError(t, err)

	// Create a directory that we can't read
	unreadableDir := filepath.Join(tmpDir, "unreadable")

	tests := []struct {
		name          string
		libName       string
		opts          []OpOption
		expectedError error
		shouldFind    bool
	}{
		{
			name:          "empty library name",
			libName:       "",
			opts:          []OpOption{WithSearchDirs(tmpDir)},
			expectedError: ErrLibraryEmpty,
			shouldFind:    false,
		},
		{
			name:          "no search dirs",
			libName:       "test.so",
			opts:          []OpOption{},
			expectedError: ErrEmptySearchDir,
			shouldFind:    false,
		},
		{
			name:          "library not found",
			libName:       "nonexistent.so",
			opts:          []OpOption{WithSearchDirs(tmpDir)},
			expectedError: ErrLibraryNotFound,
			shouldFind:    false,
		},
		{
			name:          "library found",
			libName:       "test.so",
			opts:          []OpOption{WithSearchDirs(tmpDir)},
			expectedError: nil,
			shouldFind:    true,
		},
		{
			name:    "library found with alternative name",
			libName: "test.so",
			opts: []OpOption{
				WithSearchDirs(tmpDir),
				WithAlternativeLibraryName("test.so.1"),
			},
			expectedError: nil,
			shouldFind:    true,
		},
		{
			name:    "multiple search dirs",
			libName: "test.so",
			opts: []OpOption{
				WithSearchDirs("/nonexistent", tmpDir),
			},
			expectedError: nil,
			shouldFind:    true,
		},
		{
			name:    "broken symlink",
			libName: "broken.so",
			opts: []OpOption{
				WithSearchDirs(tmpDir),
			},
			expectedError: ErrLibraryNotFound,
			shouldFind:    false,
		},
		{
			name:    "unreadable directory",
			libName: "test.so",
			opts: []OpOption{
				WithSearchDirs(unreadableDir),
			},
			expectedError: ErrLibraryNotFound,
			shouldFind:    false,
		},
		{
			name:    "multiple alternative names",
			libName: "nonexistent.so",
			opts: []OpOption{
				WithSearchDirs(tmpDir),
				WithAlternativeLibraryName("test.so"),
				WithAlternativeLibraryName("test.so.1"),
			},
			expectedError: nil,
			shouldFind:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := FindLibrary(tt.libName, tt.opts...)

			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, err)
				assert.Empty(t, path)
			} else if tt.shouldFind {
				assert.NoError(t, err)
				assert.NotEmpty(t, path)
				assert.FileExists(t, path)
			} else {
				assert.Error(t, err)
				assert.Empty(t, path)
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "file_exists_test")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	// Create a directory that we can't read
	tmpDir, err := os.MkdirTemp("", "file_exists_test_dir")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	unreadableDir := filepath.Join(tmpDir, "unreadable")
	err = os.Mkdir(unreadableDir, 0000)
	assert.NoError(t, err)
	unreadableFile := filepath.Join(unreadableDir, "file")

	tests := []struct {
		name        string
		path        string
		shouldExist bool
		wantErr     bool
	}{
		{
			name:        "existing file",
			path:        tmpFile.Name(),
			shouldExist: true,
			wantErr:     false,
		},
		{
			name:        "non-existing file",
			path:        "/path/to/nonexistent/file",
			shouldExist: false,
			wantErr:     false,
		},
		{
			name:        "permission denied",
			path:        unreadableFile,
			shouldExist: false,
			wantErr:     true,
		},
		{
			name:        "empty path",
			path:        "",
			shouldExist: false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := fileExists(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				assert.False(t, exists)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.shouldExist, exists)
			}
		})
	}
}

func TestWithSearchDirs(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  int
	}{
		{
			name:  "single path",
			paths: []string{"/path1"},
			want:  1,
		},
		{
			name:  "multiple paths",
			paths: []string{"/path1", "/path2", "/path3"},
			want:  3,
		},
		{
			name:  "empty paths",
			paths: []string{},
			want:  0,
		},
		{
			name:  "duplicate paths",
			paths: []string{"/path1", "/path1", "/path2"},
			want:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			WithSearchDirs(tt.paths...)(op)
			assert.Equal(t, tt.want, len(op.searchDirs))
		})
	}
}

func TestWithAlternativeLibraryName(t *testing.T) {
	tests := []struct {
		name      string
		altNames  []string
		wantCount int
	}{
		{
			name:      "single alternative name",
			altNames:  []string{"lib.so.1"},
			wantCount: 1,
		},
		{
			name:      "multiple alternative names",
			altNames:  []string{"lib.so.1", "lib.so.2", "lib.so.3"},
			wantCount: 3,
		},
		{
			name:      "duplicate alternative names",
			altNames:  []string{"lib.so.1", "lib.so.1"},
			wantCount: 1,
		},
		{
			name:      "empty alternative name",
			altNames:  []string{""},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			for _, name := range tt.altNames {
				WithAlternativeLibraryName(name)(op)
			}
			assert.Equal(t, tt.wantCount, len(op.alternativeLibraryNames))
		})
	}
}

func TestDirectoryExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dir_exists_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a directory that we can't read
	unreadableDir := filepath.Join(tmpDir, "unreadable")
	err = os.Mkdir(unreadableDir, 0000)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		dir         string
		shouldExist bool
		wantErr     bool
	}{
		{
			name:        "existing directory",
			dir:         tmpDir,
			shouldExist: true,
			wantErr:     false,
		},
		{
			name:        "non-existing directory",
			dir:         "/path/to/nonexistent/dir",
			shouldExist: false,
			wantErr:     false,
		},
		{
			name:        "empty path",
			dir:         "",
			shouldExist: false,
			wantErr:     false,
		},
		{
			name:        "file instead of directory",
			dir:         filepath.Join(tmpDir, "file"),
			shouldExist: false,
			wantErr:     false,
		},
	}

	// Create a regular file
	_, err = os.Create(filepath.Join(tmpDir, "file"))
	assert.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := directoryExists(tt.dir)
			if tt.wantErr {
				assert.Error(t, err)
				assert.False(t, exists)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.shouldExist, exists)
			}
		})
	}
}
