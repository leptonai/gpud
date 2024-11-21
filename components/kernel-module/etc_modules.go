package kernelmodule

import (
	"bufio"
	"bytes"
	"sort"
	"strings"
)

// ref. https://manpages.ubuntu.com/manpages/xenial/man5/modules.5.html
const DefaultEtcModulesPath = "/etc/modules"

// parseEtcModules parses the "/etc/modules" file to list the kernel modules to load at boot time.
// ref. https://manpages.ubuntu.com/manpages/xenial/man5/modules.5.html
func parseEtcModules(b []byte) ([]string, error) {
	modules := []string{}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		modules = append(modules, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Strings(modules)
	return modules, nil
}
