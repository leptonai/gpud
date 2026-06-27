// Package disk provides utilities for disk operations.
package disk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/shirou/gopsutil/v4/disk"

	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

func GetPartitions(ctx context.Context, opts ...OpOption) (Partitions, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	// When a usage command override is configured (e.g.
	// "nsenter --target 1 --mount -- df"), collect partitions and usage from that
	// command's output so the measurement runs in the host mount namespace.
	// Empty (the default) preserves the legacy gopsutil + statfs path below.
	if op.blockdevUsageCommand != "" {
		return getPartitionsWithDf(ctx, op)
	}

	partitions, err := disk.PartitionsWithContext(ctx, true)
	if err != nil {
		return nil, err
	}

	ps := make([]Partition, 0, len(partitions))
	deviceToPartitions := make(map[string]Partitions)
	for _, p := range partitions {
		if !op.matchFuncFstype(p.Fstype) {
			log.Logger.Debugw("skipping partition due to mismatch fstype", "fstype", p.Fstype, "device", p.Device, "mountPoint", p.Mountpoint)
			continue
		}
		if !op.matchFuncMountPoint(p.Mountpoint) {
			log.Logger.Debugw("skipping partition due to missing mount point", "fstype", p.Fstype, "device", p.Device, "mountPoint", p.Mountpoint)
			continue
		}

		part := Partition{
			Device:     p.Device,
			Fstype:     p.Fstype,
			MountPoint: p.Mountpoint,
		}

		timeoutCtx, cancel := context.WithTimeout(ctx, op.statTimeout)
		_, err := pkgfile.StatWithTimeout(timeoutCtx, p.Mountpoint)
		cancel()
		part.Mounted = err == nil

		if err != nil {
			if os.IsNotExist(err) {
				// e.g., deleted pod then "stat /var/lib/kubelet/pods/80017f21-3c73-48" will fail
				log.Logger.Debugw("skipping partition because mount point does not exist", "error", err, "device", part.Device, "mountPoint", part.MountPoint)
				continue
			}

			if errors.Is(err, context.DeadlineExceeded) {
				// NFS or other network filesystem might be unresponsive
				log.Logger.Warnw("stat operation timed out, marking partition as not mounted", "error", err, "device", part.Device, "mountPoint", part.MountPoint)
				part.StatTimedOut = true
			}
		}

		if part.Mounted && !op.skipUsage {
			now := time.Now()
			part.Usage, err = GetUsage(ctx, p.Mountpoint)
			took := time.Since(now)
			metricGetUsageSeconds.With(prometheus.Labels{"mount_point": p.Mountpoint}).Observe(took.Seconds())
			log.Logger.Debugw("get usage", "mountPoint", p.Mountpoint, "took", took)

			if err != nil {
				// mount point is gone
				// e.g., "no such file or directory"
				if strings.Contains(err.Error(), "no such file or directory") {
					log.Logger.Debugw("skipping partition because mount point does not exist", "error", err, "device", part.Device, "mountPoint", part.MountPoint)
					continue
				}

				return nil, fmt.Errorf("failed to get usage for mounted partition %q: %w", p.Mountpoint, err)
			}
		}

		ps = append(ps, part)

		if _, ok := deviceToPartitions[part.Device]; !ok {
			deviceToPartitions[part.Device] = make([]Partition, 0)
		}
		deviceToPartitions[part.Device] = append(deviceToPartitions[part.Device], part)
	}

	// sort in descending order of total bytes
	sort.Slice(ps, func(i, j int) bool {
		if ps[j].Usage == nil {
			return true
		}
		if ps[i].Usage == nil {
			return false
		}
		return ps[i].Usage.TotalBytes > ps[j].Usage.TotalBytes
	})

	return ps, nil
}

func GetUsage(ctx context.Context, mountPoint string) (*Usage, error) {
	usage, err := disk.UsageWithContext(ctx, mountPoint)
	if err != nil {
		return nil, err
	}
	return &Usage{
		TotalBytes: usage.Total,
		FreeBytes:  usage.Free,
		UsedBytes:  usage.Used,
	}, nil
}

type Partitions []Partition

func (parts Partitions) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "Fstype", "Mount Point", "Mounted", "Total", "Used", "Free"})

	for _, part := range parts {
		total := "n/a"
		used := "n/a"
		free := "n/a"
		if part.Usage != nil {
			total = humanize.IBytes(part.Usage.TotalBytes)
			used = humanize.IBytes(part.Usage.UsedBytes)
			free = humanize.IBytes(part.Usage.FreeBytes)
		}

		table.Append([]string{
			part.Device,
			part.Fstype,
			part.MountPoint,
			strconv.FormatBool(part.Mounted),
			total,
			used,
			free,
		})
	}

	table.Render()
}

// Returns the total bytes of all mounted partitions.
func (parts Partitions) GetMountedTotalBytes() uint64 {
	var total uint64
	for _, p := range parts {
		if p.Usage == nil {
			continue
		}

		// skip unmounted partitions
		if !p.Mounted {
			continue
		}

		total += p.Usage.TotalBytes
	}
	return total
}

type Partition struct {
	Device string `json:"device"`

	Fstype     string `json:"fstype"`
	MountPoint string `json:"mount_point"`
	Mounted    bool   `json:"mounted"`
	// StatTimedOut is true if the stat operation timed out.
	StatTimedOut bool `json:"stat_timed_out"`

	Usage *Usage `json:"usage"`
}

type Usage struct {
	TotalBytes uint64 `json:"total_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

// dfFlags are the flags gpud appends to the configured blockdev usage command.
//   - "-T" adds a filesystem Type column (needed for fstype filtering).
//   - "-B1" reports sizes in bytes (so no unit parsing is required).
//   - "-P" forces the portable POSIX format: exactly one line per filesystem,
//     which keeps the output stable and machine-parseable.
const dfFlags = "-T -B1 -P"

// getPartitionsWithDf collects partitions and usage by running the configured
// blockdev usage command (e.g. "nsenter --target 1 --mount -- df") instead of
// enumerating mounts via gopsutil (/proc/self/mountinfo) and measuring usage via
// the statfs syscall. This lets the disk component report the host's filesystems
// and their usage when gpud runs inside a container.
//
// It applies the same fstype/mount-point filters and the same descending
// total-bytes ordering as the legacy GetPartitions path, so downstream consumers
// see a consistent shape regardless of which collection path produced it.
//
// Note: usage here comes from df's "Available" column (space usable by
// unprivileged processes), which can be slightly smaller than the statfs-based
// FreeBytes (which includes filesystem-reserved blocks). This only applies when
// the override is explicitly configured; the default path is unchanged.
func getPartitionsWithDf(ctx context.Context, op *Op) (Partitions, error) {
	output, err := runDfCommand(ctx, op.blockdevUsageCommand)
	if err != nil {
		return nil, err
	}

	parsed, err := parseDfOutput(output)
	if err != nil {
		return nil, err
	}

	return filterDfPartitions(parsed, op), nil
}

// filterDfPartitions applies the same fstype/mount-point filters, mounted/usage
// semantics, and descending total-bytes ordering as the legacy GetPartitions
// path to partitions parsed from "df" output. It is separated from command
// execution so it can be unit tested without running df.
func filterDfPartitions(parsed Partitions, op *Op) Partitions {
	ps := make(Partitions, 0, len(parsed))
	for _, p := range parsed {
		if !op.matchFuncFstype(p.Fstype) {
			log.Logger.Debugw("skipping partition due to mismatch fstype", "fstype", p.Fstype, "device", p.Device, "mountPoint", p.MountPoint)
			continue
		}
		if !op.matchFuncMountPoint(p.MountPoint) {
			log.Logger.Debugw("skipping partition due to missing mount point", "fstype", p.Fstype, "device", p.Device, "mountPoint", p.MountPoint)
			continue
		}

		part := Partition{
			Device:     p.Device,
			Fstype:     p.Fstype,
			MountPoint: p.MountPoint,
			// df only lists currently mounted filesystems.
			Mounted: true,
		}
		// Mirror the legacy path's WithSkipUsage semantics: when usage is
		// skipped, leave Partition.Usage nil.
		if !op.skipUsage {
			part.Usage = p.Usage
		}

		ps = append(ps, part)
	}

	// sort in descending order of total bytes (identical to GetPartitions).
	sort.Slice(ps, func(i, j int) bool {
		if ps[j].Usage == nil {
			return true
		}
		if ps[i].Usage == nil {
			return false
		}
		return ps[i].Usage.TotalBytes > ps[j].Usage.TotalBytes
	})

	return ps
}

// runDfCommand runs "<command> -T -B1 -P" and returns its stdout. The command is
// the configured invocation prefix (gpud appends dfFlags). Only stdout is read,
// since df may emit non-fatal warnings (e.g. permission denied on a mount) to
// stderr that must not contaminate the parsed table.
func runDfCommand(ctx context.Context, command string) (string, error) {
	p, err := process.New(
		process.WithCommand(command+" "+dfFlags),
		process.WithRunAsBashScript(),
		process.WithRunBashInline(),
	)
	if err != nil {
		return "", err
	}

	if err := p.Start(ctx); err != nil {
		return "", err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	lines := make([]string, 0)
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithProcessLine(func(line string) {
			lines = append(lines, line)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return "", fmt.Errorf("failed to read df output: %w (output: %s)", err, strings.Join(lines, "\n"))
	}

	return strings.Join(lines, "\n"), nil
}

// parseDfOutput parses the output of "df -T -B1 -P". Expected columns per data
// row (POSIX format, sizes already in bytes):
//
//	Filesystem  Type  1B-blocks  Used  Available  Capacity  Mounted-on
//
// The mount point (last column) may legitimately contain spaces, so everything
// from the 7th field onward is joined back into the mount point. Rows that do
// not parse as data rows (the header, blank lines, or stray non-numeric lines)
// are skipped defensively rather than failing the whole collection.
func parseDfOutput(output string) (Partitions, error) {
	parts := make(Partitions, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 7 {
			log.Logger.Debugw("skipping df line with too few fields", "line", line)
			continue
		}

		// Skip the header row ("Filesystem Type 1B-blocks Used Available ...").
		if fields[0] == "Filesystem" {
			continue
		}

		totalBytes, errTotal := strconv.ParseUint(fields[2], 10, 64)
		usedBytes, errUsed := strconv.ParseUint(fields[3], 10, 64)
		availBytes, errAvail := strconv.ParseUint(fields[4], 10, 64)
		if errTotal != nil || errUsed != nil || errAvail != nil {
			// Not a numeric data row (e.g. a stray warning); skip defensively.
			log.Logger.Debugw("skipping df line with non-numeric size columns", "line", line)
			continue
		}

		// fields[5] is the capacity percentage (e.g. "27%"); not needed.
		mountPoint := strings.Join(fields[6:], " ")

		parts = append(parts, Partition{
			Device:     fields[0],
			Fstype:     fields[1],
			MountPoint: mountPoint,
			Mounted:    true,
			Usage: &Usage{
				TotalBytes: totalBytes,
				FreeBytes:  availBytes,
				UsedBytes:  usedBytes,
			},
		})
	}

	return parts, nil
}
