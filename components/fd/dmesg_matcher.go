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
		{check: HasVFSFileMaxLimitReached, name: eventVFSFileMaxLimitReached, message: messageVFSFileMaxLimitReached},
	}
}
