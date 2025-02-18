package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasOOM(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "basic OOM message",
			input:    "Out of memory: Killed process 123, UID 48, (httpd).",
			expected: true,
		},
		{
			name:     "timestamped OOM message",
			input:    "[Sun Dec  8 09:23:39 2024] Out of memory: Killed process 123, UID 48, (httpd).",
			expected: true,
		},
		{
			name:     "non-matching message",
			input:    "System running low on memory",
			expected: false,
		},
		{
			name:     "OOM message with score",
			input:    "Out of memory: Kill process 456 (python) score 50 or sacrifice child",
			expected: true,
		},
		{
			name:     "timestamped OOM message with extra spaces",
			input:    "[Sun Dec  8 09:23:36 2024]  Out of memory: Killed process 123, UID 48, (httpd).",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasOOM(tt.input); got != tt.expected {
				t.Errorf("HasOOM() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasOOMKillConstraint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "basic OOM kill constraint message",
			input:    "oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),",
			expected: true,
		},
		{
			name:     "timestamped OOM kill constraint message",
			input:    "[Mon Dec  9 10:24:40 2024] oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),",
			expected: true,
		},
		{
			name:     "non-matching message",
			input:    "System killed process due to memory constraints",
			expected: false,
		},
		{
			name:     "non-matching message with oom",
			input:    "[Sun Dec  8 09:23:36 2024]  Out of memory: Killed process 123, UID 48, (httpd).",
			expected: false,
		},
		{
			name:     "OOM kill constraint message with details",
			input:    "oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=cri-containerd-3fc28fa1c647ceede9f6b340d0b16c9f1f663698972d22a52e296f291638e014.scope,mems_allowed=0-1,oom_memcg=/lxc.payload.ny2g2r5hh3-lxc/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda8697f49_441d_4d4f_90d2_6d8e1fa3bbe7.slice,task_memcg=/lxc.payload.ny2g2r5hh3-lxc/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda8697f49_441d_4d4f_90d2_6d8e1fa3bbe7.slice/cri-containerd-3fc28fa1c647ceede9f6b340d0b16c9f1f663698972d22a52e296f291638e014.scope,task=node,pid=863987,uid=0",
			expected: true,
		},
		{
			name:     "OOM kill constraint message with different constraint",
			input:    "oom-kill:constraint=OTHER_CONSTRAINT",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasOOMKillConstraint(tt.input); got != tt.expected {
				t.Errorf("HasOOMKillConstraint() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasOOMKiller(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "basic invoked OOM killer message",
			input:    "postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
			expected: true,
		},
		{
			name:     "basic triggered OOM killer message",
			input:    "postgres triggered oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
			expected: true,
		},
		{
			name:     "timestamped invoked OOM killer message",
			input:    "[Tue Dec 10 11:25:41 2024] postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
			expected: true,
		},
		{
			name:     "case insensitive test",
			input:    "Process INVOKED OOM-KILLER with parameters",
			expected: true,
		},
		{
			name:     "non-matching message",
			input:    "System killed process due to memory usage",
			expected: false,
		},
		{
			name:     "OOM killer message with different process name",
			input:    "java triggered oom-killer: gfp_mask=0x14200ca, order=0, oom_score_adj=0",
			expected: true,
		},
		{
			name:     "OOM killer message with uppercase",
			input:    "PROCESS INVOKED OOM-KILLER: details here",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasOOMKiller(tt.input); got != tt.expected {
				t.Errorf("HasOOMKiller() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasOOMCgroup(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "basic memory cgroup OOM message",
			input:    "Memory cgroup out of memory: Killed process 123, UID 48, (httpd).",
			expected: true,
		},
		{
			name:     "timestamped memory cgroup OOM message",
			input:    "[Wed Dec 11 12:26:42 2024] Memory cgroup out of memory: Killed process 123, UID 48, (httpd).",
			expected: true,
		},
		{
			name:     "non-matching message",
			input:    "Process killed due to memory limits",
			expected: false,
		},
		{
			name:     "memory cgroup OOM message with score",
			input:    "Memory cgroup out of memory: Kill process 789 (nginx) score 1000 or sacrifice child",
			expected: true,
		},
		{
			name:     "detailed memory cgroup OOM message",
			input:    "Memory cgroup out of memory: Killed process 863987 (node) total-vm:42779752kB, anon-rss:41545760kB, file-rss:2836kB, shmem-rss:0kB, UID:0 pgtables:82636kB oom_score_adj:893",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasOOMCgroup(tt.input); got != tt.expected {
				t.Errorf("HasOOMCgroup() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasEDACCorrectableErrors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "basic EDAC error message",
			input:    "EDAC MC0: 1 CE memory read error",
			expected: true,
		},
		{
			name:     "detailed EDAC error message",
			input:    "EDAC MC1: 128 CE memory read error on CPU_SrcID#1_Ha#0_Chan#1_DIMM#1",
			expected: true,
		},
		{
			name:     "timestamped EDAC error message",
			input:    "[Thu Dec 12 13:27:43 2024] EDAC MC0: 1 CE memory read error",
			expected: true,
		},
		{
			name:     "non-matching message",
			input:    "Memory read error detected",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasEDACCorrectableErrors(tt.input); got != tt.expected {
				t.Errorf("HasEDACCorrectableErrors() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedName string
		expectedMsg  string
	}{
		{
			name:         "OOM basic case",
			input:        "Out of memory: Killed process 123, UID 48, (httpd).",
			expectedName: eventOOM,
			expectedMsg:  "oom detected",
		},
		{
			name:         "OOM with timestamp",
			input:        "[Sun Dec 8 09:23:39 2024] Out of memory: Killed process 123, UID 48, (httpd).",
			expectedName: eventOOM,
			expectedMsg:  "oom detected",
		},
		{
			name:         "OOM kill constraint",
			input:        "oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),",
			expectedName: eventOOMKillConstraint,
			expectedMsg:  "oom kill constraint detected",
		},
		{
			name:         "OOM killer invoked",
			input:        "postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
			expectedName: eventOOMKiller,
			expectedMsg:  "oom killer detected",
		},
		{
			name:         "OOM killer triggered",
			input:        "process triggered oom-killer: gfp_mask=0x201d2",
			expectedName: eventOOMKiller,
			expectedMsg:  "oom killer detected",
		},
		{
			name:         "OOM cgroup",
			input:        "Memory cgroup out of memory: Killed process 123, UID 48, (httpd).",
			expectedName: eventOOMCgroup,
			expectedMsg:  "oom cgroup detected",
		},
		{
			name:         "EDAC correctable error",
			input:        "EDAC MC0: 1 CE memory read error",
			expectedName: eventEDACCorrectableErrors,
			expectedMsg:  "edac correctable errors detected",
		},
		{
			name:         "EDAC correctable error with DIMM info",
			input:        "EDAC MC1: 128 CE memory read error on CPU_SrcID#1_Ha#0_Chan#1_DIMM#1",
			expectedName: eventEDACCorrectableErrors,
			expectedMsg:  "edac correctable errors detected",
		},
		{
			name:         "non-matching line",
			input:        "some random log line that doesn't match any patterns",
			expectedName: "",
			expectedMsg:  "",
		},
		{
			name:         "empty line",
			input:        "",
			expectedName: "",
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventName, message := Match(tt.input)
			assert.Equal(t, tt.expectedName, eventName)
			assert.Equal(t, tt.expectedMsg, message)
		})
	}
}
