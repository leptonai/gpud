package firmware

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

// docaHostPackages lists the DOCA host meta packages in priority order; their
// versions match the bundle format (3.2.1-044413). doca-host is left out: it
// only installs the local apt repository, so it says nothing about the
// installed DOCA software.
var docaHostPackages = []string{
	"doca-ofed",
	"doca-all",
	"doca-networking",
	"doca-roce",
	"doca-ofed-userspace",
}

// maxDpkgStatusLine bounds one line of the dpkg status file. Depends lines can
// exceed the 64KB bufio.Scanner default.
const maxDpkgStatusLine = 16 << 20

// collectDOCAHost reports the version of the first installed DOCA host
// package. It reads the dpkg database instead of running dpkg-query; hosts
// without dpkg report nothing.
func collectDOCAHost(hostFS fs.FS) []instance {
	f, err := hostFS.Open(dpkgStatusPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Logger.Debugw("skipping doca host version", "error", err)
		}
		return nil
	}
	defer func() {
		_ = f.Close()
	}()

	version, err := readDOCAHostVersion(f)
	if err != nil {
		log.Logger.Debugw("skipping doca host version", "error", err)
		return nil
	}
	if version == "" {
		return nil
	}
	return []instance{{component: apiv1.FirmwareComponentDOCAHost, version: version}}
}

// readDOCAHostVersion parses dpkg status stanzas and returns the raw Version
// of the first installed package in docaHostPackages, or "" when none is.
func readDOCAHostVersion(r io.Reader) (string, error) {
	wanted := make(map[string]struct{}, len(docaHostPackages))
	for _, pkg := range docaHostPackages {
		wanted[pkg] = struct{}{}
	}
	installed := make(map[string]string)

	var pkg, status, version string
	flush := func() {
		if _, ok := wanted[pkg]; ok && isDpkgInstalled(status) && version != "" {
			installed[pkg] = version
		}
		pkg, status, version = "", "", ""
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxDpkgStatusLine)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		// Continuation lines of multi-line fields start with a space or tab.
		if line[0] == ' ' || line[0] == '\t' {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch key {
		case "Package":
			pkg = strings.TrimSpace(value)
		case "Status":
			status = strings.TrimSpace(value)
		case "Version":
			version = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	flush()

	for _, p := range docaHostPackages {
		if v, ok := installed[p]; ok {
			return v, nil
		}
	}
	return "", nil
}

// isDpkgInstalled reports whether a dpkg Status value ("want flag status")
// marks the package as installed; "hold ok installed" counts.
func isDpkgInstalled(status string) bool {
	fields := strings.Fields(status)
	return len(fields) == 3 && fields[2] == "installed"
}
