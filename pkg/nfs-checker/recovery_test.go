package nfschecker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
)

func TestRecoveryCoverage(t *testing.T) {
	mounts, err := parseRecoveryMounts(strings.NewReader("50 1 0:50 / /nfs rw - nfs4 server:/export rw,addr=192.0.2.1\n51 1 0:51 / /other rw - nfs4 server:/other rw,addr=192.0.2.1\n"))
	require.NoError(t, err)
	req := RecoveryRequest{Servers: []string{"192.0.2.1"}, Configs: Configs{{VolumePath: "/nfs", DirName: ".check"}}}
	_, err = recoveryPaths(req, mounts)
	require.ErrorContains(t, err, "lacks a configured probe", "one healthy export cannot clear another")
	req.Configs = append(req.Configs, Config{VolumePath: "/other", DirName: ".check"})
	paths, err := recoveryPaths(req, mounts)
	require.NoError(t, err)
	require.Len(t, paths, 2)
	for _, server := range []string{"", "other", "192.0.2.2"} {
		req.Servers = []string{server}
		_, err = recoveryPaths(req, mounts)
		require.Error(t, err)
	}
	req.Servers = []string{"server"}
	_, err = recoveryPaths(req, mounts)
	require.NoError(t, err, "kernel may name source hostname instead of address")
	req.Configs[0].VolumePath = "/local"
	_, err = recoveryPaths(req, mounts)
	require.Error(t, err)
}

func TestRecoveryMountParsing(t *testing.T) {
	mounts, err := parseRecoveryMounts(strings.NewReader("50 1 0:50 / /host/nfs\\040space rw shared:2 - nfs4 [2001:db8::1]:/export rw,addr=2001:db8::1\n51 1 0:51 / /local rw - ext4 /dev/sda rw\n"))
	require.NoError(t, err)
	require.Len(t, mounts, 1)
	require.True(t, mounts[0].matches("2001:db8::1"))
	req := RecoveryRequest{HostRoot: "/host", Servers: []string{"2001:db8::1"}, Configs: Configs{{VolumePath: "/nfs space", DirName: ".check"}}}
	_, err = recoveryPaths(req, mounts)
	require.NoError(t, err)
	_, err = recoveryPaths(req, append(mounts, mounts[0]))
	require.ErrorContains(t, err, "one exact NFS mount")
	for _, dir := range []string{"", "../escape", "/absolute"} {
		req.Configs[0].DirName = dir
		_, err = recoveryPaths(req, mounts)
		require.Error(t, err)
	}
	_, err = parseRecoveryMounts(strings.NewReader("malformed\n"))
	require.Error(t, err)
	changed := append([]recoveryMount(nil), mounts...)
	changed[0].identity = "new mount ID"
	require.False(t, equalRecoveryMounts(mounts, changed))
}

func TestRecoveryProbeIOAndConfinement(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux mount IDs")
	}
	dir := t.TempDir()
	f, err := os.Open(dir)
	require.NoError(t, err)
	data, err := os.ReadFile(fmt.Sprintf("/proc/self/fdinfo/%d", f.Fd()))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	var id string
	for _, line := range strings.Split(string(data), "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok && key == "mnt_id" {
			id = strings.TrimSpace(value)
		}
	}
	require.NotEmpty(t, id)
	path := recoveryPath{mount: recoveryMount{path: dir, identity: id + " mount"}, dir: ".check"}
	require.NoError(t, probeRecoveryPath(context.Background(), path))
	entries, err := os.ReadDir(filepath.Join(dir, ".check"))
	require.NoError(t, err)
	require.Empty(t, entries, "probe cleans its own unique file")
	path.mount.identity = "invalid mount"
	require.ErrorContains(t, probeRecoveryPath(context.Background(), path), "expected NFS mount")
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "escape")))
	path.dir = "escape"
	require.Error(t, probeRecoveryPath(context.Background(), path))
	entries, err = os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRecoveryRunnerDeadlineAndSingleFlight(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})
	runner := &RecoveryRunner{probe: func(context.Context, RecoveryRequest) error {
		close(started)
		<-unblock // Model a syscall/cleanup that ignores cancellation.
		return nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, RecoveryRequest{}) }()
	<-started
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("caller blocked on canceled I/O")
	}
	require.True(t, runner.IsRunning(), "slot includes canceled I/O and cleanup")
	for range 10 {
		require.ErrorContains(t, runner.Run(context.Background(), RecoveryRequest{}), "still running")
	}
	close(unblock)
	require.Eventually(t, func() bool { return !runner.IsRunning() }, time.Second, time.Millisecond)
	// The previous worker's late success must not be reused by this check.
	freshErr := errors.New("fresh probe failed")
	runner.probe = func(context.Context, RecoveryRequest) error { return freshErr }
	require.ErrorIs(t, runner.Run(context.Background(), RecoveryRequest{}), freshErr)
	runner.probe = func(context.Context, RecoveryRequest) error { return nil }
	require.NoError(t, runner.Run(context.Background(), RecoveryRequest{}))
}

func TestRecoveryRunnerCanceledBeforeStart(t *testing.T) {
	runner := &RecoveryRunner{probe: func(context.Context, RecoveryRequest) error { t.Error("unexpected probe"); return nil }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, runner.Run(ctx, RecoveryRequest{}), context.Canceled)
	require.False(t, runner.running)
}

func TestRecoveryProbeWorkflow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux mount IDs")
	}
	for _, mode := range []string{"success", "mount changed", "inventory error", "post inventory error", "probe error"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			f, err := os.Open(dir)
			require.NoError(t, err)
			data, err := os.ReadFile(fmt.Sprintf("/proc/self/fdinfo/%d", f.Fd()))
			require.NoError(t, err)
			require.NoError(t, f.Close())
			var id string
			for _, line := range strings.Split(string(data), "\n") {
				if key, value, ok := strings.Cut(line, ":"); ok && key == "mnt_id" {
					id = strings.TrimSpace(value)
				}
			}
			require.NotEmpty(t, id)
			req := RecoveryRequest{Servers: []string{"server"}, Configs: Configs{{VolumePath: dir, DirName: ".check"}}}
			if mode == "probe error" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".check"), []byte("not a directory"), 0600))
			}
			calls := 0
			// Use the real local mount ID to exercise I/O and control inventory changes.
			// This is a workflow test, not a substitute for a live NFS outage test.
			readMounts := func() ([]recoveryMount, error) {
				calls++
				if mode == "inventory error" || (mode == "post inventory error" && calls == 2) {
					return nil, errors.New("read failed")
				}
				identity := id + " fixture"
				if mode == "mount changed" && calls == 2 {
					identity = "replacement"
				}
				return []recoveryMount{{path: dir, source: "server:/export", identity: identity}}, nil
			}
			err = runRecoveryProbe(context.Background(), req, readMounts)
			if mode == "success" {
				require.NoError(t, err)
				require.Equal(t, 2, calls)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestRecoveryProbeCancellationSkipsIO(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	dir := t.TempDir()
	req := RecoveryRequest{Servers: []string{"server"}, Configs: Configs{{VolumePath: dir, DirName: ".check"}}}
	err := runRecoveryProbe(ctx, req, func() ([]recoveryMount, error) {
		cancel()
		return []recoveryMount{{path: dir, source: "server:/export"}}, nil
	})
	require.ErrorIs(t, err, context.Canceled)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestRecoveryRunnerOwnsRequestAfterTimeout(t *testing.T) {
	started, unblock := make(chan struct{}), make(chan struct{})
	observed := make(chan RecoveryRequest, 1)
	runner := &RecoveryRunner{probe: func(_ context.Context, req RecoveryRequest) error {
		close(started)
		<-unblock
		observed <- req
		return nil
	}}
	req := RecoveryRequest{Configs: Configs{{VolumePath: "/original"}}, Servers: []string{"original"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, req) }()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	req.Configs[0].VolumePath = "/replacement"
	req.Servers[0] = "replacement"
	close(unblock)
	got := <-observed
	require.Equal(t, "/original", got.Configs[0].VolumePath)
	require.Equal(t, "original", got.Servers[0])
}

func TestRecoveryServerAddressNormalization(t *testing.T) {
	mount := recoveryMount{source: "server:/export", address: "2001:db8::1"}
	require.True(t, mount.matches("[2001:0db8:0000:0000:0000:0000:0000:0001]"))
	require.False(t, mount.matches("2001:db8::2"))
	mount.address = "192.0.2.1"
	require.True(t, mount.matches("::ffff:192.0.2.1"))
}

func TestRecoveryRejectsConflictingDirectories(t *testing.T) {
	req := RecoveryRequest{Servers: []string{"server"}, Configs: Configs{
		{VolumePath: "/nfs", DirName: ".one"},
		{VolumePath: "/nfs", DirName: ".two"},
	}}
	_, err := recoveryPaths(req, []recoveryMount{{path: "/nfs", source: "server:/export"}})
	require.ErrorContains(t, err, "conflicting")
}

func TestRecoveryIOFailuresNeverEstablishHealth(t *testing.T) {
	for _, operation := range []string{"write", "fsync", "seek", "read", "mismatch", "close", "remove", "cancel after fsync"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dir := t.TempDir()
			path := recoveryPath{mount: recoveryMount{path: dir}, dir: ".check"}
			// Mount validation is covered separately; exercise actual local file I/O
			// with one failed operation, including cleanup after the failed probe.
			identity := mockey.Mock(checkRecoveryFileMount).Return(nil).Build()
			defer identity.UnPatch()
			injected := errors.New("injected " + operation + " failure")
			isProbe := func(f *os.File) bool { return strings.Contains(f.Name(), ".gpud-recovery-") }
			var patch *mockey.Mocker
			var leaked *os.File
			switch operation {
			case "write":
				patch = mockey.Mock((*os.File).Write).When(func(f *os.File, _ []byte) bool { return isProbe(f) }).Return(0, injected).Build()
			case "fsync":
				patch = mockey.Mock((*os.File).Sync).When(isProbe).Return(injected).Build()
			case "seek":
				patch = mockey.Mock((*os.File).Seek).When(func(f *os.File, _ int64, _ int) bool { return isProbe(f) }).Return(int64(0), injected).Build()
			case "read":
				patch = mockey.Mock((*os.File).Read).When(func(f *os.File, _ []byte) bool { return isProbe(f) }).Return(0, injected).Build()
			case "mismatch":
				patch = mockey.Mock((*os.File).Read).When(func(f *os.File, _ []byte) bool { return isProbe(f) }).Return(0, io.EOF).Build()
			case "close":
				patch = mockey.Mock((*os.File).Close).When(isProbe).To(func(f *os.File) error { leaked = f; return injected }).Build()
			case "remove":
				patch = mockey.Mock((*os.Root).Remove).When(func(_ *os.Root, name string) bool { return strings.Contains(name, ".gpud-recovery-") }).Return(injected).Build()
			case "cancel after fsync":
				patch = mockey.Mock((*os.File).Sync).When(isProbe).To(func(*os.File) error { cancel(); return nil }).Build()
			}
			defer func() {
				patch.UnPatch()
				if leaked != nil {
					_ = leaked.Close()
				}
			}()
			err := probeRecoveryPath(ctx, path)
			switch operation {
			case "mismatch":
				require.ErrorContains(t, err, "readback mismatch")
			case "cancel after fsync":
				require.ErrorIs(t, err, context.Canceled)
			default:
				require.ErrorIs(t, err, injected)
			}
			entries, readErr := os.ReadDir(filepath.Join(dir, ".check"))
			require.NoError(t, readErr)
			if operation != "remove" {
				require.Empty(t, entries, "cleanup must run even on failure or cancellation")
			}
		})
	}
}

func TestRecoveryRunnerProductionPathRejectsMissingCoverage(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux mount inventory")
	}
	runner := &RecoveryRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.ErrorContains(t, runner.Run(ctx, RecoveryRequest{}), "requires mapped servers")
	require.False(t, runner.IsRunning())
}

func TestRecoveryRunnerCancellationWinsOverSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &RecoveryRunner{probe: func(context.Context, RecoveryRequest) error { cancel(); return nil }}
	require.ErrorIs(t, runner.Run(ctx, RecoveryRequest{}), context.Canceled)
}

func TestRecoveryInventoryDetectsLocalOvermounts(t *testing.T) {
	original := "50 1 0:50 / /parent/nfs rw - nfs4 server:/export rw\n"
	before, err := parseRecoveryMounts(strings.NewReader(original))
	require.NoError(t, err)
	for _, path := range []string{"/parent/nfs", "/parent", "/parent/nfs/.check"} {
		after, err := parseRecoveryMounts(strings.NewReader(original + "51 1 0:51 / " + path + " rw - tmpfs tmpfs rw\n"))
		require.NoError(t, err)
		require.False(t, equalRecoveryMounts(before, after), "must detect local overmount at %s", path)
	}
	unrelated, err := parseRecoveryMounts(strings.NewReader(original + "51 1 0:51 / /parent/nfs-other rw - tmpfs tmpfs rw\n"))
	require.NoError(t, err)
	require.True(t, equalRecoveryMounts(before, unrelated), "unrelated mount churn must not prevent recovery")
}

func TestRecoveryRejectsReplacementBeforeCreatingDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux mount identity")
	}
	dir := t.TempDir()
	err := probeRecoveryPath(t.Context(), recoveryPath{mount: recoveryMount{path: dir, identity: "invalid mount"}, dir: ".check"})
	require.ErrorContains(t, err, "expected NFS mount")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "do not create a checker directory on a replacement filesystem")
}
