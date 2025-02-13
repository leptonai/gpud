package memory

import (
	"regexp"
)

const (
	// e.g.,
	// Out of memory: Killed process 123, UID 48, (httpd).
	// [...] Out of memory: Killed process 123, UID 48, (httpd).
	//
	// NOTE: this is often followed by a line like:
	// [Sun Dec  8 09:23:39 2024] oom_reaper: reaped process 345646 (vector), now anon-rss:0kB, file-rss:0kB, shmem-rss:0
	// (to reap the memory used by the OOM victim)
	EventOOM   = "memory_oom"
	RegexOOM   = `Out of memory:`
	MessageOOM = `oom detected`

	// e.g.,
	// oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),
	// [...] oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),
	EventOOMKillConstraint   = "memory_oom_kill_constraint"
	RegexOOMKillConstraint   = `oom-kill:constraint=`
	MessageOOMKillConstraint = "oom kill constraint detected"

	// e.g.,
	// postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0
	// [...] postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0
	EventOOMKiller   = "memory_oom_killer"
	RegexOOMKiller   = `(?i)\b(invoked|triggered) oom-killer\b`
	MessageOOMKiller = "oom killer detected"

	// e.g.,
	// Memory cgroup out of memory: Killed process 123, UID 48, (httpd).
	// [...] Memory cgroup out of memory: Killed process 123, UID 48, (httpd).
	EventOOMCgroup   = "memory_oom_cgroup"
	RegexOOMCgroup   = `Memory cgroup out of memory`
	MessageOOMCgroup = "oom cgroup detected"

	// May indicate that Dual Inline Memory Module (DIMM) is beginning to fail.
	//
	// e.g.,
	// [...] EDAC MC0: 1 CE memory read error
	// [...] EDAC MC1: 128 CE memory read error on CPU_SrcID#1_Ha#0_Chan#1_DIMM#1
	//
	// ref.
	// https://serverfault.com/questions/682909/how-to-find-faulty-memory-module-from-mce-message
	// https://github.com/Azure/azurehpc/blob/2d57191cb35ed638525ba9424cc2aa1b5abe1c05/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L51C20-L51C40
	EventEDACCorrectableErrors   = "memory_edac_correctable_errors"
	RegexEDACCorrectableErrors   = `.*CE memory read error.*`
	MessageEDACCorrectableErrors = "edac correctable errors detected"
)

var (
	compiledOOM                   = regexp.MustCompile(RegexOOM)
	compiledOOMKillConstraint     = regexp.MustCompile(RegexOOMKillConstraint)
	compiledOOMKiller             = regexp.MustCompile(RegexOOMKiller)
	compiledOOMCgroup             = regexp.MustCompile(RegexOOMCgroup)
	compiledEDACCorrectableErrors = regexp.MustCompile(RegexEDACCorrectableErrors)
)

// Returns true if the line indicates that the file-max limit has been reached.
// ref. https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
func HasOOM(line string) bool {
	if match := compiledOOM.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func HasOOMKillConstraint(line string) bool {
	if match := compiledOOMKillConstraint.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func HasOOMKiller(line string) bool {
	if match := compiledOOMKiller.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func HasOOMCgroup(line string) bool {
	if match := compiledOOMCgroup.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func HasEDACCorrectableErrors(line string) bool {
	if match := compiledEDACCorrectableErrors.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

func Match(line string) (name string, message string) {
	for _, m := range getMatches() {
		if m.check(line) {
			return m.name, m.message
		}
	}
	return "", ""
}

type match struct {
	check   func(string) bool
	name    string
	message string
}

func getMatches() []match {
	return []match{
		{check: HasOOM, name: EventOOM, message: MessageOOM},
		{check: HasOOMKillConstraint, name: EventOOMKillConstraint, message: MessageOOMKillConstraint},
		{check: HasOOMKiller, name: EventOOMKiller, message: MessageOOMKiller},
		{check: HasOOMCgroup, name: EventOOMCgroup, message: MessageOOMCgroup},
		{check: HasEDACCorrectableErrors, name: EventEDACCorrectableErrors, message: MessageEDACCorrectableErrors},
	}
}
