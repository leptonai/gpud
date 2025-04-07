package systemd

import (
	"os/exec"
)

func JournalctlExists() bool {
	p, err := exec.LookPath("journalctl")
	if err != nil {
		return false
	}
	return p != ""
}
