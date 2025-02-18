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
	eventOOM   = "memory_oom"
	regexOOM   = `Out of memory:`
	messageOOM = `oom detected`

	// e.g.,
	// oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),
	// [...] oom-kill:constraint=CONSTRAINT_MEMCG,nodemask=(null),
	eventOOMKillConstraint   = "memory_oom_kill_constraint"
	regexOOMKillConstraint   = `oom-kill:constraint=`
	messageOOMKillConstraint = "oom kill constraint detected"

	// e.g.,
	// postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0
	// [...] postgres invoked oom-killer: gfp_mask=0x201d2, order=0, oomkilladj=0
	eventOOMKiller   = "memory_oom_killer"
	regexOOMKiller   = `(?i)\b(invoked|triggered) oom-killer\b`
	messageOOMKiller = "oom killer detected"

	// e.g.,
	// Memory cgroup out of memory: Killed process 123, UID 48, (httpd).
	// [...] Memory cgroup out of memory: Killed process 123, UID 48, (httpd).
	eventOOMCgroup   = "memory_oom_cgroup"
	regexOOMCgroup   = `Memory cgroup out of memory`
	messageOOMCgroup = "oom cgroup detected"

	// May indicate that Dual Inline Memory Module (DIMM) is beginning to fail.
	//
	// e.g.,
	// [...] EDAC MC0: 1 CE memory read error
	// [...] EDAC MC1: 128 CE memory read error on CPU_SrcID#1_Ha#0_Chan#1_DIMM#1
	//
	// ref.
	// https://serverfault.com/questions/682909/how-to-find-faulty-memory-module-from-mce-message
	// https://github.com/Azure/azurehpc/blob/2d57191cb35ed638525ba9424cc2aa1b5abe1c05/experimental/aks_npd_draino/npd/deployment/node-problem-detector-config.yaml#L51C20-L51C40
	eventEDACCorrectableErrors   = "memory_edac_correctable_errors"
	regexEDACCorrectableErrors   = `.*CE memory read error.*`
	messageEDACCorrectableErrors = "edac correctable errors detected"
)

var (
	compiledOOM                   = regexp.MustCompile(regexOOM)
	compiledOOMKillConstraint     = regexp.MustCompile(regexOOMKillConstraint)
	compiledOOMKiller             = regexp.MustCompile(regexOOMKiller)
	compiledOOMCgroup             = regexp.MustCompile(regexOOMCgroup)
	compiledEDACCorrectableErrors = regexp.MustCompile(regexEDACCorrectableErrors)
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

func Match(line string) (eventName string, regex string, message string) {
	for _, m := range getMatches() {
		if m.check(line) {
			return m.eventName, m.regex, m.message
		}
	}
	return "", "", ""
}

type match struct {
	check     func(string) bool
	eventName string
	regex     string
	message   string
}

func getMatches() []match {
	return []match{
		{check: HasOOM, eventName: eventOOM, regex: regexOOM, message: messageOOM},
		{check: HasOOMKillConstraint, eventName: eventOOMKillConstraint, regex: regexOOMKillConstraint, message: messageOOMKillConstraint},
		{check: HasOOMKiller, eventName: eventOOMKiller, regex: regexOOMKiller, message: messageOOMKiller},
		{check: HasOOMCgroup, eventName: eventOOMCgroup, regex: regexOOMCgroup, message: messageOOMCgroup},
		{check: HasEDACCorrectableErrors, eventName: eventEDACCorrectableErrors, regex: regexEDACCorrectableErrors, message: messageEDACCorrectableErrors},
	}
}
