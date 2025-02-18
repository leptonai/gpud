package nccl

import "testing"

func TestHasNCCLSegfaultInLibnccl(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "exact match with full details",
			line: "[Thu Oct 10 03:06:53 2024] pt_main_thread[2536443]: segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2[7f7c7ac00000+d3d3000]",
			want: true,
		},
		{
			name: "basic match without timestamp",
			line: "segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so",
			want: true,
		},
		{
			name: "match with different timestamp format",
			line: "[123123213213] segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so",
			want: true,
		},
		{
			name: "match with ISO timestamp",
			line: "kern  :err   : 2025-02-10T16:28:06,502716+00:00 segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2",
			want: true,
		},
		{
			name: "no match - different library",
			line: "segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libcuda.so",
			want: false,
		},
		{
			name: "no match - not a segfault",
			line: "some other error in libnccl.so",
			want: false,
		},
		{
			name: "empty string",
			line: "",
			want: false,
		},
		{
			name: "partial match - missing segfault",
			line: "error in libnccl.so",
			want: false,
		},
		{
			name: "partial match - missing library",
			line: "segfault at 7f797fe00000 ip 00007f7c7ac69996",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasNCCLSegfaultInLibnccl(tt.line); got != tt.want {
				t.Errorf("HasNCCLSegfaultInLibnccl(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantName    string
		wantMessage string
	}{
		{
			name:        "NCCL segfault with full details",
			line:        "[Thu Oct 10 03:06:53 2024] pt_main_thread[2536443]: segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2[7f7c7ac00000+d3d3000]",
			wantName:    EventNCCLSegfaultInLibnccl,
			wantMessage: messageNCCLSegfaultInLibnccl,
		},
		{
			name:        "NCCL segfault with ISO timestamp",
			line:        "kern  :err   : 2025-02-10T16:28:06,502716+00:00 segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2",
			wantName:    EventNCCLSegfaultInLibnccl,
			wantMessage: messageNCCLSegfaultInLibnccl,
		},
		{
			name:        "no match - different library",
			line:        "segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libcuda.so",
			wantName:    "",
			wantMessage: "",
		},
		{
			name:        "no match - not a segfault",
			line:        "some other error in libnccl.so",
			wantName:    "",
			wantMessage: "",
		},
		{
			name:        "empty string",
			line:        "",
			wantName:    "",
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotMessage := Match(tt.line)
			if gotName != tt.wantName {
				t.Errorf("Match() name = %v, want %v", gotName, tt.wantName)
			}
			if gotMessage != tt.wantMessage {
				t.Errorf("Match() message = %v, want %v", gotMessage, tt.wantMessage)
			}
		})
	}
}

func TestGetMatches(t *testing.T) {
	matches := getMatches()

	// Verify we have the expected number of matchers
	if len(matches) != 1 {
		t.Errorf("getMatches() returned %d matches, want 1", len(matches))
	}

	// Verify the NCCL segfault matcher
	ncclMatch := matches[0]
	if ncclMatch.name != EventNCCLSegfaultInLibnccl {
		t.Errorf("first match name = %v, want %v", ncclMatch.name, EventNCCLSegfaultInLibnccl)
	}
	if ncclMatch.message != messageNCCLSegfaultInLibnccl {
		t.Errorf("first match message = %v, want %v", ncclMatch.message, messageNCCLSegfaultInLibnccl)
	}

	// Test the check function
	if !ncclMatch.check("[Thu Oct 10 03:06:53 2024] pt_main_thread[2536443]: segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2[7f7c7ac00000+d3d3000]") {
		t.Error("check function failed to match valid input")
	}
	if ncclMatch.check("invalid input") {
		t.Error("check function matched invalid input")
	}
}
