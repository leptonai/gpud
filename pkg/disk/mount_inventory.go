package disk

import (
	"context"
	"encoding/json"
	"fmt"
)

// GetMounts reads mount metadata without stat or statfs calls on the mounted
// filesystems. Mounted means present in the mount table, not responsive.
// Usage is always nil. The findmnt override selects the same namespace used by
// the other disk collectors when gpud runs in a container.
func GetMounts(ctx context.Context, opts ...OpOption) (Partitions, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}
	command := op.findmntCommand
	if command == "" {
		command = "findmnt"
	}
	// Do not use --df or request size columns: those trigger filesystem I/O.
	command += " --kernel --list --json --nocanonicalize --nofsroot --output SOURCE,FSTYPE,TARGET"
	parts, err := readMounts(ctx, command)
	if err != nil {
		return nil, err
	}
	filtered := make(Partitions, 0, len(parts))
	for _, part := range parts {
		if op.matchFuncFstype(part.Fstype) && op.matchFuncMountPoint(part.MountPoint) {
			filtered = append(filtered, part)
		}
	}
	return filtered, nil
}

// FindMountSource resolves a path in the command's namespace without requesting
// filesystem usage. Path resolution itself may require filesystem I/O, so the
// caller must provide a deadline. Unlike an in-process stat, the subprocess can
// be abandoned when that deadline expires.
func FindMountSource(ctx context.Context, target, command string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("empty mount target")
	}
	if command == "" {
		command = "findmnt"
	}
	parts, err := readMounts(ctx, command+" --kernel --list --json --nofsroot --output SOURCE,FSTYPE,TARGET --target "+quoteShellArgument(target))
	if err != nil {
		return "", err
	}
	// findmnt emits mount-table order for stacked mounts. Use the last
	// reported source (the topmost mount), allowing duplicate self-binds.
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i].Device != "" {
			return parts[i].Device, nil
		}
	}
	return "", fmt.Errorf("no mount source for %q", target)
}

func readMounts(ctx context.Context, command string) (Partitions, error) {
	output, err := runDiskCommand(ctx, command)
	if err != nil {
		return nil, fmt.Errorf("failed to read mount inventory: %w", err)
	}
	var table struct {
		Filesystems []struct {
			Source string `json:"source"`
			Fstype string `json:"fstype"`
			Target string `json:"target"`
		} `json:"filesystems"`
	}
	if err := json.Unmarshal([]byte(output), &table); err != nil {
		return nil, fmt.Errorf("failed to parse mount inventory: %w", err)
	}
	if table.Filesystems == nil {
		return nil, fmt.Errorf("missing filesystems in mount inventory")
	}
	parts := make(Partitions, 0, len(table.Filesystems))
	for _, mount := range table.Filesystems {
		parts = append(parts, Partition{Device: mount.Source, Fstype: mount.Fstype, MountPoint: mount.Target, Mounted: true})
	}
	return parts, nil
}
