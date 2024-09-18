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
			name:  "OOMConstraint",
			regex: EventOOMKillConstraintRegex,
			matches: []string{
				"oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=cri-containerd-3fc28fa1c647ceede9f6b340d0b16c9f1f663698972d22a52e296f291638e014.scope,mems_allowed=0-1,oom_memcg=/lxc.payload.ny2g2r5hh3-lxc/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda8697f49_441d_4d4f_90d2_6d8e1fa3bbe7.slice,task_memcg=/lxc.payload.ny2g2r5hh3-lxc/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda8697f49_441d_4d4f_90d2_6d8e1fa3bbe7.slice/cri-containerd-3fc28fa1c647ceede9f6b340d0b16c9f1f663698972d22a52e296f291638e014.scope,task=node,pid=863987,uid=0",
				"oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),cpuset=cri-containerd-3fc28fa1c647ceede9f6b340d0b16c9f1f663698972d22a52e296f291638e014.scope,mems_allowed=0-1,oom_memcg=/lxc.payload.ny2g2r5hh3-lxc/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda8697f49_441d_4d4f_90d2_6d8e1fa3bbe7.slice,task_memcg=/lxc.payload.ny2g2r5hh3-lxc/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-poda8697f49_441d_4d4f_90d2_6d8e1fa3bbe7.slice/cri-containerd-3fc28fa1c647ceede9f6b340d0b16c9f1f663698972d22a52e296f291638e014.scope,task=pt_main_thread,pid=1483257,uid=0",
			},
			notMatch: []string{
				"oom-kill:abc=OTHER_CONSTRAINT",
				"Memory constraint reached",
			},
		},
		{
			name:  "OOMKiller",
			regex: EventOOMKillerRegex,
			matches: []string{
				"postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0",
				"java triggered oom-killer: gfp_mask=0x14200ca, order=0, oom_score_adj=0",
				"PROCESS INVOKED OOM-KILLER: details here",
				"pt_main_thread invoked oom-killer: gfp_mask=0x400dc0(GFP_KERNEL_ACCOUNT|__GFP_ZERO), order=0, oom_score_adj=893",
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
				"Memory cgroup out of memory: Killed process 863987 (node) total-vm:42779752kB, anon-rss:41545760kB, file-rss:2836kB, shmem-rss:0kB, UID:0 pgtables:82636kB oom_score_adj:893",
				"Memory cgroup out of memory: Killed process 1483257 (pt_main_thread) total-vm:48979828kB, anon-rss:23167252kB, file-rss:83700kB, shmem-rss:20kB, UID:0 pgtables:46968kB oom_score_adj:893",
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
