package nfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		wantEventName string
		wantMessage   string
	}{
		// ── Rule A: server not responding ──
		{
			name:          "server not responding, timed out",
			line:          "nfs: server 7.247.192.16 not responding, timed out",
			wantEventName: eventNFSServerNotResponding,
			wantMessage:   messageNFSServerNotResponding + ": 7.247.192.16",
		},
		{
			name:          "server not responding, still trying",
			line:          "nfs: server nfs-prod-01.example.com not responding, still trying",
			wantEventName: eventNFSServerNotResponding,
			wantMessage:   messageNFSServerNotResponding + ": nfs-prod-01.example.com",
		},
		{
			name:          "server not responding with kernel timestamp prefix",
			line:          "kernel: nfs: server 10.0.0.5 not responding, timed out",
			wantEventName: eventNFSServerNotResponding,
			wantMessage:   messageNFSServerNotResponding + ": 10.0.0.5",
		},

		// ── Rule A pair: server OK ──
		{
			name:          "server OK",
			line:          "nfs: server 7.247.192.16 OK",
			wantEventName: eventNFSServerOK,
			wantMessage:   messageNFSServerOK + ": 7.247.192.16",
		},
		{
			name:          "server OK with hostname",
			line:          "nfs: server nfs-prod-01.example.com OK",
			wantEventName: eventNFSServerOK,
			wantMessage:   messageNFSServerOK + ": nfs-prod-01.example.com",
		},

		// ── Rule B: lock reclaim failed ──
		{
			name:          "lock reclaim failed",
			line:          "nfs4_reclaim_open_state: Lock reclaim failed!",
			wantEventName: eventNFSLockReclaimFailed,
			wantMessage:   messageNFSLockReclaimFailed,
		},
		{
			name:          "lock reclaim failed without exclamation",
			line:          "nfs4_reclaim_open_state: Lock reclaim failed",
			wantEventName: eventNFSLockReclaimFailed,
			wantMessage:   messageNFSLockReclaimFailed,
		},

		// ── Rule C: writeback hang stack frames ──
		// All real dmesg stack-trace lines start with a leading space, which
		// pkg/kmsg's parser preserves. These cases enforce that the regex
		// boundary anchor accepts leading whitespace.
		{
			name:          "writeback hang nfs_lock_and_join_requests with leading space",
			line:          " nfs_lock_and_join_requests+0x61/0x2b0 [nfs]",
			wantEventName: eventNFSWritebackHang,
			wantMessage:   messageNFSWritebackHang,
		},
		{
			name:          "writeback hang nfs_wb_all with leading space",
			line:          " nfs_wb_all+0x2c/0x190 [nfs]",
			wantEventName: eventNFSWritebackHang,
			wantMessage:   messageNFSWritebackHang,
		},
		{
			name:          "writeback hang nfs_page_async_flush with leading space",
			line:          " nfs_page_async_flush+0x24/0x290 [nfs]",
			wantEventName: eventNFSWritebackHang,
			wantMessage:   messageNFSWritebackHang,
		},
		{
			name:          "writeback hang nfs_writepages_callback with tab indent",
			line:          "\tnfs_writepages_callback+0x31/0x60 [nfs]",
			wantEventName: eventNFSWritebackHang,
			wantMessage:   messageNFSWritebackHang,
		},
		{
			name:          "writeback hang at line start (no leading whitespace)",
			line:          "nfs_lock_and_join_requests+0x61/0x2b0 [nfs]",
			wantEventName: eventNFSWritebackHang,
			wantMessage:   messageNFSWritebackHang,
		},

		// ── Negative cases ──
		{
			name:          "negative: server starting is not not-responding",
			line:          "nfs: server 7.247.192.16 starting",
			wantEventName: "",
			wantMessage:   "",
		},
		{
			name:          "negative: unrelated lock message",
			line:          "some other syscall lock contention detected",
			wantEventName: "",
			wantMessage:   "",
		},
		{
			name:          "negative: similar but unrelated nfs symbol",
			line:          " nfs_xxx_other+0x12/0x340 [nfs]",
			wantEventName: "",
			wantMessage:   "",
		},
		{
			name:          "negative: writeback symbol as substring of larger token",
			line:          " prefix_nfs_lock_and_join_requests+0x61/0x2b0 [nfs]",
			wantEventName: "",
			wantMessage:   "",
		},
		{
			name:          "negative: empty line",
			line:          "",
			wantEventName: "",
			wantMessage:   "",
		},
		{
			name:          "negative: nfs mount info",
			line:          "NFS: Registering the id_resolver key type",
			wantEventName: "",
			wantMessage:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEventName, gotMessage := Match(tt.line)
			assert.Equal(t, tt.wantEventName, gotEventName, "Match() eventName")
			assert.Equal(t, tt.wantMessage, gotMessage, "Match() message")
		})
	}
}
