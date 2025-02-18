package fd

import (
	"regexp"
)

const (
	// e.g.,
	// [...] VFS: file-max limit 1000000 reached
	// [...] VFS: file-max limit <number> reached
	//
	// ref.
	// https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
	eventVFSFileMaxLimitReached   = "vfs_file_max_limit_reached"
	regexVFSFileMaxLimitReached   = `VFS: file-max limit \d+ reached`
	messageVFSFileMaxLimitReached = "VFS file-max limit reached"
)

var compiledVFSFileMaxLimitReached = regexp.MustCompile(regexVFSFileMaxLimitReached)

// Returns true if the line indicates that the file-max limit has been reached.
// ref. https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
func HasVFSFileMaxLimitReached(line string) bool {
	if match := compiledVFSFileMaxLimitReached.FindStringSubmatch(line); match != nil {
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
		{check: HasVFSFileMaxLimitReached, eventName: eventVFSFileMaxLimitReached, regex: regexVFSFileMaxLimitReached, message: messageVFSFileMaxLimitReached},
	}
}
