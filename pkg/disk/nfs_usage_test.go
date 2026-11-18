package disk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
)

func usageWork(t *testing.T, c *NFSUsageCollector, path string) *nfsUsageWork {
	t.Helper()
	var w *nfsUsageWork
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		w = c.pending["\x00"+path]
		return w != nil
	}, time.Second, time.Millisecond)
	return w
}

func TestNFSUsageBlockedMountDoesNotFreezeCollection(t *testing.T) {
	blocked := make(chan struct{})
	started := make(chan struct{})
	var slowCalls atomic.Int32
	c := &NFSUsageCollector{probe: func(_ context.Context, part Partition, _ string) (*Usage, error) {
		if part.MountPoint == "/slow" {
			if slowCalls.Add(1) == 1 {
				close(started)
			}
			<-blocked
		}
		return &Usage{TotalBytes: 100, FreeBytes: 60, UsedBytes: 40}, nil
	}}
	mounts := Partitions{{MountPoint: "/slow", Device: "server:/a", Fstype: "nfs4"}, {MountPoint: "/fast", Device: "server:/b", Fstype: "nfs4"}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan Partitions, 1)
	go func() { result <- c.Collect(ctx, mounts, "") }()
	slow := usageWork(t, c, "/slow")
	fast := usageWork(t, c, "/fast")
	<-fast.done
	<-started
	cancel()
	parts := <-result
	require.True(t, parts[0].StatTimedOut)
	require.Nil(t, parts[0].Usage)
	require.True(t, parts[1].Mounted, "a ready result survives waiting for the slow mount")
	require.EqualValues(t, 100, parts[1].Usage.TotalBytes)
	for range 5 {
		parts = c.Collect(t.Context(), mounts, "")
		require.True(t, parts[0].StatTimedOut)
		require.NotNil(t, parts[1].Usage)
	}
	require.EqualValues(t, 1, slowCalls.Load(), "no accumulating statfs workers")
	close(blocked)
	<-slow.done
	require.ErrorIs(t, slow.err, context.Canceled, "late successful I/O is discarded")
	parts = c.Collect(t.Context(), mounts, "")
	require.False(t, parts[0].StatTimedOut)
	require.NotNil(t, parts[0].Usage)
	require.EqualValues(t, 2, slowCalls.Load(), "recovery requires a fresh call")
}

func TestNFSUsageFailuresAndDuplicateTargets(t *testing.T) {
	for _, err := range []error{errors.New("permission denied"), context.DeadlineExceeded} {
		c := &NFSUsageCollector{probe: func(context.Context, Partition, string) (*Usage, error) { return nil, err }}
		parts := c.Collect(t.Context(), Partitions{{MountPoint: "/nfs", Device: "old"}, {MountPoint: "/nfs", Device: "new"}}, "")
		require.Len(t, parts, 1, "one health sample per mount path")
		require.Equal(t, "new", parts[0].Device)
		require.False(t, parts[0].Mounted)
		if errors.Is(err, context.DeadlineExceeded) {
			require.True(t, parts[0].StatTimedOut)
		} else {
			require.Equal(t, err.Error(), parts[0].StatError)
		}
	}
	c := &NFSUsageCollector{probe: func(context.Context, Partition, string) (*Usage, error) { return nil, nil }}
	parts := c.Collect(t.Context(), Partitions{{MountPoint: "/nfs"}}, "")
	require.NotEmpty(t, parts[0].StatError)
}

func TestNFSUsageWorkerCapAndCancellation(t *testing.T) {
	blocked := make(chan struct{})
	var calls atomic.Int32
	c := &NFSUsageCollector{probe: func(context.Context, Partition, string) (*Usage, error) {
		calls.Add(1)
		<-blocked
		return &Usage{}, nil
	}}
	mounts := make(Partitions, maxNFSUsageWorkers+5)
	for i := range mounts {
		mounts[i].MountPoint = fmt.Sprintf("/nfs/%d", i)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan Partitions, 1)
	go func() { result <- c.Collect(ctx, mounts, "") }()
	require.Eventually(t, func() bool { return calls.Load() == maxNFSUsageWorkers }, time.Second, time.Millisecond)
	cancel()
	for _, p := range <-result {
		require.True(t, p.StatTimedOut)
	}
	require.EqualValues(t, maxNFSUsageWorkers, calls.Load())
	close(blocked)
	for _, m := range mounts {
		<-usageWork(t, c, m.MountPoint).done
	}
}

func TestNFSUsageDefaultProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := probeNFSUsage(ctx, Partition{MountPoint: "/"}, "")
	require.ErrorIs(t, err, context.Canceled)
	if runtime.GOOS != "linux" {
		u, err := probeNFSUsage(t.Context(), Partition{MountPoint: "/"}, "")
		require.NoError(t, err)
		require.NotNil(t, u)
	}
	c := &NFSUsageCollector{}
	parts := c.Collect(ctx, Partitions{{MountPoint: "/"}}, "")
	require.True(t, parts[0].StatTimedOut)
}

func TestNFSUsageCommandRejectsReplacementFilesystem(t *testing.T) {
	part := Partition{MountPoint: "/nfs", Device: "server:/export", Fstype: "nfs4"}
	for _, row := range []string{
		"server:/export nfs4 100 40 60 40% /nfs",
		"/dev/root ext4 100 40 60 40% /",
		"/dev/root ext4 100 40 60 40% /nfs",
		"other:/export nfs4 100 40 60 40% /nfs",
	} {
		t.Run(row, func(t *testing.T) {
			command := filepath.Join(t.TempDir(), "df")
			require.NoError(t, os.WriteFile(command, []byte("#!/bin/sh\nprintf '%s\\n' '"+row+"'\n"), 0700))
			usage, err := probeNFSUsage(t.Context(), part, "sh '"+command+"'")
			if strings.HasPrefix(row, "server:") {
				require.NoError(t, err)
				require.NotNil(t, usage)
			} else {
				require.ErrorContains(t, err, "identity changed")
				require.Nil(t, usage)
			}
		})
	}
}

func TestNFSUsageNativeMountIdentity(t *testing.T) {
	part := Partition{MountPoint: "/nfs space", Device: "server:/export", Fstype: "nfs4"}
	original := "50 1 0:50 / /nfs\\040space rw - nfs4 server:/export rw\n"
	identity, err := nfsUsageIdentity(strings.NewReader(original), part, "50")
	require.NoError(t, err)
	require.Equal(t, "50", identity)
	for _, changed := range []string{"", "invalid", strings.Replace(original, "nfs4", "ext4", 1), original + "51 1 0:51 / /nfs\\040space rw - ext4 /dev/root rw\n"} {
		_, err = nfsUsageIdentity(strings.NewReader(changed), part, "50")
		require.Error(t, err)
	}
}

func TestNFSUsageIdentityDiscardsPartialMountinfo(t *testing.T) {
	part := Partition{MountPoint: "/tools", Device: "server:/export", Fstype: "nfs4"}
	const record = "50 1 0:50 / /tools rw - nfs4 server:/export rw\n"
	readErr := errors.New("mountinfo read interrupted")
	reader := io.MultiReader(strings.NewReader(record), iotest.ErrReader(readErr))
	identity, err := nfsUsageIdentity(reader, part, "50")
	require.ErrorIs(t, err, readErr)
	require.Empty(t, identity)
}

func TestNFSUsageIdentitySurvivesOptionChanges(t *testing.T) {
	part := Partition{MountPoint: "/tools", Fstype: "nfs4", Device: "server:/export"}
	for _, record := range []string{
		"50 1 0:50 / /tools rw shared:7 - nfs4 server:/export rw,vers=4.1",
		"50 1 0:50 / /tools ro - nfs4 server:/export ro,vers=4.1",
	} {
		identity, err := nfsUsageIdentity(strings.NewReader(record), part, "50")
		require.NoError(t, err)
		require.Equal(t, "50", identity)
	}
}

func TestNFSUsageIdentityPreservesUnicodeWhitespace(t *testing.T) {
	// Linux uses ASCII space as its field separator and preserves nonbreaking spaces.
	part := Partition{MountPoint: "/shared\u00a0tools", Fstype: "nfs4", Device: "server:/export\u00a0one"}
	const record = "50 1 0:50 / /shared\u00a0tools rw - nfs4 server:/export\u00a0one rw"
	identity, err := nfsUsageIdentity(strings.NewReader(record), part, "50")
	require.NoError(t, err)
	require.Equal(t, "50", identity)
}

func TestNFSUsageAutofsMountIdentity(t *testing.T) {
	part := Partition{MountPoint: "/tools", Fstype: "nfs", Device: "server:/export"}
	const trigger = "100 34 0:62 / /tools rw,relatime shared:806 - autofs auto.tools rw,indirect\n"
	const export = "3950 905 0:531 / /tools ro,relatime shared:2042 - nfs server:/export ro,vers=3\n"
	const local = "4100 3950 0:70 / /tools rw - tmpfs tmpfs rw\n"
	for _, tc := range []struct {
		name, records, visibleID string
		wantUsage                bool
	}{
		{"trigger first", trigger + export, "3950", true},
		{"export first", export + trigger, "3950", true},
		{"expired export", trigger, "100", false},
		{"covered export", trigger + export, "100", false},
		{"replaced export", trigger + strings.Replace(export, "server:/export", "server:/other", 1), "3950", false},
		{"local overmount", trigger + export + local, "4100", false},
		{"two real mounts", export + strings.Replace(export, "3950", "3951", 1), "3951", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity, err := nfsUsageIdentity(strings.NewReader(tc.records), part, tc.visibleID)
			if tc.wantUsage {
				require.NoError(t, err)
				require.Equal(t, tc.visibleID, identity)
			} else {
				require.Error(t, err)
				require.Empty(t, identity)
			}
		})
	}
}
