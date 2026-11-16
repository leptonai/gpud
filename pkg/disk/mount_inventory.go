package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leptonai/gpud/pkg/process"
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
	p, err := process.New(process.WithCommand(command), process.WithRunAsBashScript(), process.WithRunBashInline())
	if err != nil {
		return nil, err
	}
	if err := p.Start(ctx); err != nil {
		return nil, err
	}
	defer func() { _ = p.Close(ctx) }()
	var output strings.Builder
	if err := process.Read(ctx, p, process.WithReadStdout(), process.WithWaitForCmd(), process.WithProcessLine(func(line string) {
		output.WriteString(line)
		output.WriteByte('\n')
	})); err != nil {
		return nil, fmt.Errorf("failed to read mount inventory: %w", err)
	}
	var table struct {
		Filesystems []struct {
			Source string `json:"source"`
			Fstype string `json:"fstype"`
			Target string `json:"target"`
		} `json:"filesystems"`
	}
	if err := json.Unmarshal([]byte(output.String()), &table); err != nil {
		return nil, fmt.Errorf("failed to parse mount inventory: %w", err)
	}
	parts := make(Partitions, 0, len(table.Filesystems))
	for _, mount := range table.Filesystems {
		if !op.matchFuncFstype(mount.Fstype) || !op.matchFuncMountPoint(mount.Target) {
			continue
		}
		parts = append(parts, Partition{Device: mount.Source, Fstype: mount.Fstype, MountPoint: mount.Target, Mounted: true})
	}
	return parts, nil
}
