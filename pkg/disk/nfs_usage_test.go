package disk

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/mockey"
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
	// Root-device aliases vary between findmnt and mountinfo on CI hosts.
	// Identity validation has dedicated fixtures; exercise real usage I/O here.
	inventory := mockey.Mock(readNFSUsageIdentity).Return("stable mount", nil).Build()
	defer inventory.UnPatch()
	u, err := probeNFSUsage(t.Context(), Partition{MountPoint: "/"}, "")
	require.NoError(t, err)
	require.NotNil(t, u)
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
	identity, err := nfsUsageIdentity(strings.NewReader(original), part)
	require.NoError(t, err)
	require.Contains(t, identity, "50 1")
	for _, changed := range []string{"", "invalid", strings.Replace(original, "nfs4", "ext4", 1), original + "51 1 0:51 / /nfs\\040space rw - ext4 /dev/root rw\n"} {
		_, err = nfsUsageIdentity(strings.NewReader(changed), part)
		require.Error(t, err)
	}
	calls := 0
	inventory := mockey.Mock(readNFSUsageIdentity).To(func(Partition) (string, error) { calls++; return fmt.Sprint(calls), nil }).Build()
	defer inventory.UnPatch()
	usage := mockey.Mock(GetUsage).Return(&Usage{TotalBytes: 100}, nil).Build()
	defer usage.UnPatch()
	if runtime.GOOS == "linux" {
		_, err = probeNFSUsage(t.Context(), part, "")
		require.ErrorContains(t, err, "identity changed")
	}
}
