package nfschecker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// All operations run synchronously inside RecoveryRunner's single worker.
// Do not use per-operation timeout goroutines: those can outlive their caller
// and would release the worker slot while NFS I/O is still outstanding.
func runRecoveryProbe(ctx context.Context, req RecoveryRequest, readMounts func() ([]recoveryMount, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mounts, err := readMounts()
	if err != nil {
		return err
	}
	paths, err := recoveryPaths(req, mounts)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := probeRecoveryPath(ctx, path); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	after, err := readMounts()
	if err != nil {
		return err
	}
	// A successful write to a replacement/local mount is not NFS recovery.
	// Recheck the full NFS mount identities, including mount ID, after all I/O.
	if !equalRecoveryMounts(mounts, after) {
		return errors.New("NFS mounts changed during recovery probe")
	}
	return nil
}

type recoveryMount struct {
	path     string
	source   string
	address  string
	identity string
}

func readRecoveryMounts() ([]recoveryMount, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return parseRecoveryMounts(f)
}

func parseRecoveryMounts(r io.Reader) ([]recoveryMount, error) {
	var mounts []recoveryMount
	var records []struct{ path, line string }
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for scanner.Scan() {
		line := scanner.Text()
		left, right, ok := strings.Cut(line, " - ")
		a, b := strings.Fields(left), strings.Fields(right)
		if !ok || len(a) < 6 || len(b) < 3 {
			return nil, errors.New("invalid mountinfo record")
		}
		records = append(records, struct{ path, line string }{unescape.Replace(a[4]), line})
		if b[0] != "nfs" && b[0] != "nfs4" {
			continue
		}
		m := recoveryMount{path: unescape.Replace(a[4]), source: unescape.Replace(b[1]), identity: line}
		for _, opt := range strings.Split(b[2], ",") {
			if addr, ok := strings.CutPrefix(opt, "addr="); ok {
				m.address = addr
			}
		}
		mounts = append(mounts, m)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	// A local overmount leaves the underlying NFS record in mountinfo. Include
	// overlapping mounts so replacement during I/O cannot look like recovery.
	for i := range mounts {
		identity := []string{mounts[i].identity}
		for _, record := range records {
			if record.line != mounts[i].identity && recoveryPathsOverlap(record.path, mounts[i].path) {
				identity = append(identity, record.line)
			}
		}
		mounts[i].identity = strings.Join(identity, "\n")
	}
	return mounts, nil
}

func recoveryPathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, strings.TrimRight(b, "/")+"/") || strings.HasPrefix(b, strings.TrimRight(a, "/")+"/")
}

func equalRecoveryMounts(a, b []recoveryMount) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (m recoveryMount) matches(server string) bool {
	host, _, ok := strings.Cut(m.source, ":/")
	if !ok {
		return false
	}
	server = strings.Trim(server, "[]")
	return server != "" && (sameNFSServer(host, server) || sameNFSServer(m.address, server))
}

func sameNFSServer(a, b string) bool {
	a, b = strings.Trim(a, "[]"), strings.Trim(b, "[]")
	if aa, err := netip.ParseAddr(a); err == nil {
		bb, err := netip.ParseAddr(b)
		return err == nil && aa.Unmap() == bb.Unmap()
	}
	return strings.EqualFold(a, b)
}

type recoveryPath struct {
	mount recoveryMount
	dir   string
}

func recoveryPaths(req RecoveryRequest, mounts []recoveryMount) ([]recoveryPath, error) {
	if len(req.Servers) == 0 || len(req.Configs) == 0 {
		return nil, errors.New("NFS recovery requires mapped servers and configured writable mounts")
	}
	configured := make(map[string]string)
	for _, cfg := range req.Configs {
		cfg.HostRoot = req.HostRoot
		if !filepath.IsAbs(cfg.VolumePath) || cfg.DirName == "" || filepath.IsAbs(cfg.DirName) || filepath.Clean(cfg.DirName) == ".." || strings.HasPrefix(filepath.Clean(cfg.DirName), ".."+string(filepath.Separator)) {
			return nil, errors.New("invalid NFS recovery probe path")
		}
		path := filepath.Clean(cfg.resolvedVolumePath())
		if dir, exists := configured[path]; exists && dir != cfg.DirName {
			return nil, fmt.Errorf("conflicting NFS probe directories for %q", path)
		}
		configured[path] = cfg.DirName
	}
	// Every visible mount for a warned server must be covered. Success on one
	// export cannot prove another export on the same server has recovered. Never
	// create probe files on mounts the operator has not configured for writes.
	for _, server := range req.Servers {
		matched := false
		for _, mount := range mounts {
			if !mount.matches(server) {
				continue
			}
			matched = true
			if _, ok := configured[mount.path]; !ok {
				return nil, fmt.Errorf("NFS recovery lacks a configured probe for %q", mount.path)
			}
		}
		if !matched {
			return nil, fmt.Errorf("NFS recovery cannot map server %q to a mount", server)
		}
	}
	paths := make([]recoveryPath, 0, len(configured))
	for path, dir := range configured {
		count := 0
		var selected recoveryMount
		for _, mount := range mounts {
			if mount.path == path {
				count++
				selected = mount
			}
		}
		if count != 1 {
			return nil, fmt.Errorf("NFS recovery requires one exact NFS mount at %q", path)
		}
		paths = append(paths, recoveryPath{mount: selected, dir: dir})
	}
	// Stable ordering makes failures reproducible within the shared time budget.
	slices.SortFunc(paths, func(a, b recoveryPath) int { return strings.Compare(a.mount.path, b.mount.path) })
	return paths, nil
}

func probeRecoveryPath(ctx context.Context, path recoveryPath) (retErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Root confines directory creation and symlink resolution to the configured
	// mount. Otherwise a checker directory symlink could test a local filesystem.
	root, err := os.OpenRoot(path.mount.path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	// Verify the opened root before creating anything on a replacement mount.
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	identityErr := checkRecoveryFileMount(directory, path.mount.identity)
	if err := errors.Join(identityErr, directory.Close()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.MkdirAll(path.dir, 0755); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	name := filepath.Join(path.dir, ".gpud-recovery-"+rand.Text())
	f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Remove(name)) }()
	// Cleanup stays in this worker even after cancellation. Releasing its slot
	// before Close/Remove finish would permit overlapping blocked operations.
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	if err := ctx.Err(); err != nil {
		return err
	}
	// Verify the actual opened file's mount, not just its pathname. Nested local
	// mounts and concurrent unmounts must never produce evidence of NFS recovery.
	if err := checkRecoveryFileMount(f, path.mount.identity); err != nil {
		return err
	}
	payload := []byte(name)
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := f.Write(payload); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// fsync is the extra evidence absent from the original write/read checker:
	// the server must acknowledge this fresh write, not just local cached data.
	if err := f.Sync(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(len(payload)+1)))
	if err != nil {
		return err
	}
	if !bytes.Equal(data, payload) {
		return errors.New("NFS recovery probe readback mismatch")
	}
	return nil
}

func checkRecoveryFileMount(f *os.File, identity string) error {
	data, err := os.ReadFile(fmt.Sprintf("/proc/self/fdinfo/%d", f.Fd()))
	if err != nil {
		return err
	}
	fields := strings.Fields(identity)
	if len(fields) == 0 {
		return errors.New("missing NFS mount identity")
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, val, ok := strings.Cut(line, ":")
		if ok && key == "mnt_id" && strings.TrimSpace(val) == fields[0] {
			return nil
		}
	}
	return errors.New("recovery probe file is not on the expected NFS mount")
}
