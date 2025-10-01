package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"

	"github.com/dustin/go-humanize"
)

// Runs "findmnt --target [TARGET] --json --df" and parses the output.
func FindMnt(ctx context.Context, target string) (*FindMntOutput, error) {
	findmntPath, err := file.LocateExecutable("findmnt")
	if err != nil {
		return nil, err
	}

	p, err := process.New(
		process.WithCommand(fmt.Sprintf("%s --target %s --json --df", findmntPath, target)),
		process.WithRunAsBashScript(),
		process.WithRunBashInline(),
	)
	if err != nil {
		return nil, err
	}

	if err := p.Start(ctx); err != nil {
		return nil, err
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
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			lines = append(lines, line)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		return nil, fmt.Errorf("failed to read findmnt output: %w (output: %s)", err, strings.Join(lines, "\n"))
	}

	out, err := ParseFindMntOutput(strings.Join(lines, "\n"))
	if err != nil {
		return nil, err
	}
	out.Target = target
	return out, nil
}

// Represents the output of the command
// "findmnt --target /var/lib/kubelet --json --df".
// ref. https://man7.org/linux/man-pages/man8/findmnt.8.html
type FindMntOutput struct {
	// The input mount target.
	Target string `json:"target"`

	Filesystems []FoundMnt `json:"filesystems"`
}

type FoundMnt struct {
	// Regardless of the input mount target, this is where the target is mounted.
	MountedPoint string `json:"mounted_point"`

	// The filesystem may use more block devices.
	// This is why findmnt provides  SOURCE and SOURCES (pl.) columns.
	// ref. https://man7.org/linux/man-pages/man8/findmnt.8.html
	Sources []string `json:"sources"`

	Fstype string `json:"fstype"`

	SizeHumanized string `json:"size_humanized"`
	SizeBytes     uint64 `json:"size_bytes"`

	UsedHumanized string `json:"used_humanized"`
	UsedBytes     uint64 `json:"used_bytes"`

	AvailableHumanized string `json:"available_humanized"`
	AvailableBytes     uint64 `json:"available_bytes"`

	UsedPercentHumanized string  `json:"used_percent_humanized"`
	UsedPercent          float64 `json:"used_percent"`
}

type rawFindMntOutput struct {
	Filesystems []rawFoundMnt `json:"filesystems"`
}

type rawFoundMnt struct {
	Target string `json:"target"`

	// The filesystem may use more block devices.
	// This is why findmnt provides  SOURCE and SOURCES (pl.) columns.
	// ref. https://man7.org/linux/man-pages/man8/findmnt.8.html
	Source string `json:"source"`

	Fstype string `json:"fstype"`
	Size   string `json:"size"`
	Used   string `json:"used"`
	Avail  string `json:"avail"`
	UseP   string `json:"use%"`
}

func ParseFindMntOutput(output string) (*FindMntOutput, error) {
	var raw rawFindMntOutput
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, err
	}
	o := &FindMntOutput{}
	for _, rawMntOutput := range raw.Filesystems {
		parsedSize, err := humanize.ParseBytes(rawMntOutput.Size)
		if err != nil {
			return nil, err
		}

		parsedUsed, err := humanize.ParseBytes(rawMntOutput.Used)
		if err != nil {
			return nil, err
		}

		parsedAvail, err := humanize.ParseBytes(rawMntOutput.Avail)
		if err != nil {
			return nil, err
		}

		usePFloat, err := strconv.ParseFloat(strings.TrimSuffix(rawMntOutput.UseP, "%"), 64)
		if err != nil {
			return nil, err
		}

		o.Filesystems = append(o.Filesystems, FoundMnt{
			MountedPoint: rawMntOutput.Target,
			Sources:      extractMntSources(rawMntOutput.Source),
			Fstype:       rawMntOutput.Fstype,

			SizeHumanized: rawMntOutput.Size,
			SizeBytes:     parsedSize,

			UsedHumanized: rawMntOutput.Used,
			UsedBytes:     parsedUsed,

			AvailableHumanized: rawMntOutput.Avail,
			AvailableBytes:     parsedAvail,

			UsedPercentHumanized: rawMntOutput.UseP,
			UsedPercent:          usePFloat,
		})
	}
	return o, nil
}

// extractMntSources extracts mount sources from the findmnt source output.
//
// e.g.,
// "/dev/mapper/vgroot-lvroot[/var/lib/lxc/ny2g2r14hh2-lxc/rootfs]"
// becomes
// ["/dev/mapper/vgroot-lvroot", "/var/lib/lxc/ny2g2r14hh2-lxc/rootfs"]
//
// e.g.,
// "/dev/mapper/lepton_vg-lepton_lv[/kubelet]"
// becomes
// ["/dev/mapper/lepton_vg-lepton_lv", "/kubelet"]
func extractMntSources(input string) []string {
	src := strings.TrimSuffix(input, "]")
	sources := make([]string, 0)
	for _, s := range strings.Split(src, "[") {
		if s == "" {
			continue
		}
		for _, ss := range strings.Split(s, ",") {
			sources = append(sources, strings.TrimSpace(ss))
		}
	}
	return sources
}
