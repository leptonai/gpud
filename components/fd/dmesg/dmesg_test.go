package dmesg

import "testing"

func TestHasVFSFileMaxLimitReached(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{line: "VFS: file-max limit 1000000 reached", want: true},
		{line: "[Sun Dec  1 14:54:40 2024] VFS: file-max limit 1000000 reached", want: true},
		{line: "some other log message", want: false},
		{line: "", want: false},
	}
	for _, tt := range tests {
		if got := HasVFSFileMaxLimitReached(tt.line); got != tt.want {
			t.Errorf("HasVFSFileMaxLimitReached(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}
