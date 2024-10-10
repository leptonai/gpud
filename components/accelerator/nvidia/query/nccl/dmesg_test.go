package nccl

import (
	"regexp"
	"testing"
)

func TestRegexSegfaultInLibnccl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		log     string
		matches bool
	}{
		{"[Thu Oct 10 03:06:53 2024] pt_main_thread[2536443]: segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so.2[7f7c7ac00000+d3d3000]", true},
		{"segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so", true},
		{"[123123213213] segfault at 7f797fe00000 ip 00007f7c7ac69996 sp 00007f7c12fd7c30 error 4 in libnccl.so", true},
	}

	re, err := regexp.Compile(RegexSegfaultInLibnccl)
	if err != nil {
		t.Fatalf("Error compiling regex: %v", err)
	}
	for _, test := range tests {
		matched := re.MatchString(test.log)
		if matched != test.matches {
			t.Errorf("Expected match: %v, got: %v for log: %s", test.matches, matched, test.log)
		}
	}
}
