package dmesg

import (
	"regexp"
	"testing"
)

func TestOOMRegexes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		regex    string
		matches  []string
		notMatch []string
	}{
		{
			name:  "OOMKill",
			regex: EventOOMKillRegex,
			matches: []string{
				"Out of memory: Killed process 123, UID 48, (httpd).",
				"Out of memory: Kill process 456 (python) score 50 or sacrifice child",
			},
			notMatch: []string{
				"System running low on memory",
				"Process terminated due to insufficient resources",
			},
		},
		{
			name:  "OOMKiller",
			regex: EventOOMKillerRegex,
			matches: []string{
				"postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
				"java triggered oom-killer: gfp_mask=0x14200ca, order=0, oom_score_adj=0",
				"PROCESS INVOKED OOM-KILLER: details here",
			},
			notMatch: []string{
				"System killed process due to low memory",
				"OOM situation detected",
			},
		},
		{
			name:  "OOMCgroup",
			regex: EventOOMCgroupRegex,
			matches: []string{
				"Memory cgroup out of memory: Killed process 123, UID 48, (httpd).",
				"Memory cgroup out of memory: Kill process 789 (nginx) score 1000 or sacrifice child",
			},
			notMatch: []string{
				"Cgroup memory limit reached",
				"Out of memory error in container",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.regex)

			for _, s := range tt.matches {
				if !re.MatchString(s) {
					t.Errorf("Expected regex %s to match: %s", tt.regex, s)
				}
			}

			for _, s := range tt.notMatch {
				if re.MatchString(s) {
					t.Errorf("Expected regex %s not to match: %s", tt.regex, s)
				}
			}
		})
	}
}
