package nfs

import "regexp"

const (
	// Rule A (main-line): Linux NFS client signals the server is hung or the
	// network is broken.
	// e.g.,
	// nfs: server 7.247.192.16 not responding, timed out
	eventNFSServerNotResponding   = "nfs_server_not_responding"
	regexNFSServerNotResponding   = `nfs: server [^ ]+ not responding`
	messageNFSServerNotResponding = "NFS server not responding"

	// Recovery signal, paired with eventNFSServerNotResponding to cancel out
	// transient network blips.
	// e.g.,
	// nfs: server 7.247.192.16 OK
	eventNFSServerOK   = "nfs_server_ok"
	regexNFSServerOK   = `nfs: server [^ ]+ OK`
	messageNFSServerOK = "NFS server recovered"

	// Rule B: NFS lock state manager failed to reclaim state from server.
	// e.g.,
	// nfs4_reclaim_open_state: Lock reclaim failed!
	eventNFSLockReclaimFailed   = "nfs_lock_reclaim_failed"
	regexNFSLockReclaimFailed   = `nfs4_reclaim_open_state: Lock reclaim failed`
	messageNFSLockReclaimFailed = "NFS lock reclaim failed (state manager unable to recover)"

	// Rule C: NFS writeback path is stuck spinning in the kernel — fingerprint
	// of the 2026-05-15 nsc-svg-slurm-1-gpu-136 NFS hang. When softlockup /
	// hung_task / panic dumps a stack, lines like the following appear:
	//   nfs_lock_and_join_requests+0x61/0x2b0 [nfs]
	//   nfs_wb_all+0x2c/0x190 [nfs]
	//   nfs_page_async_flush+0x24/0x290 [nfs]
	//   nfs_writepages_callback+0x31/0x60 [nfs]
	//
	// Stack frames typically have a leading space; pkg/kmsg preserves it
	// (strings.SplitN(line, ";", 2), no trim). The regex therefore anchors on
	// either the start of the line or a whitespace boundary, so we don't match
	// substrings of unrelated symbols.
	eventNFSWritebackHang   = "nfs_writeback_hang"
	regexNFSWritebackHang   = `(^|\s)(nfs_lock_and_join_requests|nfs_wb_all|nfs_page_async_flush|nfs_writepages_callback)\+0x[0-9a-f]+`
	messageNFSWritebackHang = "NFS client stuck in kernel writeback path"
)

var (
	compiledNFSServerNotResponding = regexp.MustCompile(regexNFSServerNotResponding)
	compiledNFSServerOK            = regexp.MustCompile(regexNFSServerOK)
	compiledNFSLockReclaimFailed   = regexp.MustCompile(regexNFSLockReclaimFailed)
	compiledNFSWritebackHang       = regexp.MustCompile(regexNFSWritebackHang)
)

// HasNFSServerNotResponding reports whether the line indicates an NFS server
// became unresponsive.
func HasNFSServerNotResponding(line string) bool {
	return compiledNFSServerNotResponding.MatchString(line)
}

// HasNFSServerOK reports whether the line indicates an NFS server recovered.
func HasNFSServerOK(line string) bool {
	return compiledNFSServerOK.MatchString(line)
}

// HasNFSLockReclaimFailed reports whether the line indicates NFS lock reclaim
// (state manager recovery) failed.
func HasNFSLockReclaimFailed(line string) bool {
	return compiledNFSLockReclaimFailed.MatchString(line)
}

// HasNFSWritebackHang reports whether the line is a kernel stack frame
// indicating the NFS writeback path is stuck.
func HasNFSWritebackHang(line string) bool {
	return compiledNFSWritebackHang.MatchString(line)
}

// Match returns the first NFS hang related kernel-message match for the given
// log line. It is a kmsg.MatchFunc.
func Match(line string) (eventName string, message string) {
	for _, m := range getMatches() {
		if m.check(line) {
			return m.eventName, m.message
		}
	}
	return "", ""
}

type match struct {
	check     func(string) bool
	eventName string
	regex     string
	message   string
}

func getMatches() []match {
	return []match{
		{check: HasNFSServerNotResponding, eventName: eventNFSServerNotResponding, regex: regexNFSServerNotResponding, message: messageNFSServerNotResponding},
		{check: HasNFSServerOK, eventName: eventNFSServerOK, regex: regexNFSServerOK, message: messageNFSServerOK},
		{check: HasNFSLockReclaimFailed, eventName: eventNFSLockReclaimFailed, regex: regexNFSLockReclaimFailed, message: messageNFSLockReclaimFailed},
		{check: HasNFSWritebackHang, eventName: eventNFSWritebackHang, regex: regexNFSWritebackHang, message: messageNFSWritebackHang},
	}
}
