package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/dustin/go-humanize"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	nvidia_nccl "github.com/leptonai/gpud/components/accelerator/nvidia/nccl"
	nvidia_peermem "github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	nvidia_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	nvidiatemperaturecomponent "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	nvidia_xid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	cpucomponent "github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/fd"
	memorycomponent "github.com/leptonai/gpud/components/memory"
	oscomponent "github.com/leptonai/gpud/components/os"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/fuse"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	latency_edge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const (
	inProgress  = "\033[33m⌛\033[0m"
	checkMark   = "\033[32m✔\033[0m"
	warningSign = "\033[31m✘\033[0m"
)

func printSummary(result components.HealthStateCheckResult) {
	header := checkMark
	if result.HealthState() != apiv1.StateTypeHealthy {
		header = warningSign
	}
	fmt.Printf("%s %s\n", header, result.Summary())
	fmt.Println(result.String())
}

var defaultCheckHealthStateFuncs = []func(context.Context) (components.HealthStateCheckResult, error){
	oscomponent.CheckHealthState,
	cpucomponent.CheckHealthState,
	memorycomponent.CheckHealthState,
}

// Runs the scan operations.
func Scan(ctx context.Context, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	fmt.Printf("\n\n%s scanning the host (GOOS %s)\n\n", inProgress, runtime.GOOS)

	for _, checkFunc := range defaultCheckHealthStateFuncs {
		rs, err := checkFunc(ctx)
		if err != nil {
			return err
		}
		printSummary(rs)
	}

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		log.Logger.Warnw("error checking nvidia gpu installation", "error", err)
		return err
	}

	if nvidiaInstalled {
		fmt.Printf("\n%s scanning nvidia accelerators\n", inProgress)

		nvmlInstance, err := nvidianvml.NewInstanceV2()
		if err != nil {
			return err
		}
		nvidiaTempData, err := nvidiatemperaturecomponent.CheckHealthState(nvmlInstance)
		if err != nil {
			return err
		}
		printSummary(nvidiaTempData)

		for lib, alternatives := range nvidia_query.DefaultNVIDIALibraries {
			opts := []file.OpOption{
				file.WithSearchDirs(nvidia_query.DefaultNVIDIALibrariesSearchDirs...),
			}
			for _, alt := range alternatives {
				opts = append(opts, file.WithAlternativeLibraryName(alt))
			}
			libPath, err := file.FindLibrary(lib, opts...)
			if err != nil {
				log.Logger.Warnw("error finding library", "library", lib, "error", err)
			} else {
				fmt.Printf("%s found library %s at %s\n", checkMark, lib, libPath)
			}
		}

		output, err := nvidia_query.Get(ctx)
		if err != nil {
			return err
		}

		output.PrintInfo(nvidia_query.WithDebug(op.debug))

		if op.checkInfiniband {
			fmt.Printf("\n%s checking nvidia infiniband ports/rates\n", inProgress)
			threshold, err := infiniband.SupportsInfinibandPortRate(output.GPUProductName())
			if err != nil {
				log.Logger.Warnw("error getting infiniband port rate", "error", err)
			} else {
				atLeastPorts := threshold.AtLeastPorts
				atLeastRate := threshold.AtLeastRate
				if err := infiniband.CheckInfiniband(ctx, op.ibstatCommand, threshold); err != nil {
					fmt.Printf("%s ibstat ports/rates check failed (at least ports: %d, rate: %v) (%s)\n", warningSign, atLeastPorts, atLeastRate, err)
				} else {
					fmt.Printf("%s ibstat ports/rates check passed (at least ports: %d, rate: %v)\n", checkMark, atLeastPorts, atLeastRate)
				}
			}
		} else {
			fmt.Printf("%s skipped ibstat check (infiniband class not found or ibstat not found)\n", checkMark)
		}
	}
	println()

	if op.kmsgCheck {
		if os.Geteuid() != 0 {
			return errors.New("requires sudo/root access in order to scan kernel message errors")
		}

		fmt.Printf("%s scanning kernel messages\n", inProgress)
		issueCount, err := scanKmsg(ctx)
		if err != nil {
			return err
		}
		if issueCount == 0 {
			fmt.Printf("%s scanned kernel messages -- found no issue\n", checkMark)
		} else {
			fmt.Printf("%s scanned kernel messages -- found %d issue(s)\n", warningSign, issueCount)
		}
	}

	if op.netcheck {
		fmt.Printf("\n%s checking network connectivity to edge/derp servers\n", inProgress)
		latencies, err := latency_edge.Measure(ctx, latency_edge.WithVerbose(op.debug))
		if err != nil {
			log.Logger.Warnw("error measuring latencies", "error", err)
		} else {
			latencies.RenderTable(os.Stdout)
			fmt.Printf("\n\n%s latency check complete\n\n", checkMark)
		}
	}

	if op.diskcheck {
		fmt.Printf("\n%s checking disk\n", inProgress)
		partitions, err := disk.GetPartitions(ctx, disk.WithFstype(disk.DefaultMatchFuncFstype))
		if err != nil {
			log.Logger.Warnw("error getting partitions", "error", err)
		} else {
			if len(partitions) > 0 {
				fmt.Printf("\npartitions have total mounted size %s\n", humanize.Bytes(partitions.GetMountedTotalBytes()))
			}
			partitions.RenderTable(os.Stdout)
			println()
		}

		blockDevices, err := disk.GetBlockDevices(ctx, disk.WithDeviceType(disk.DefaultMatchFuncDeviceType))
		if err != nil {
			log.Logger.Warnw("error getting block devices", "error", err)
		} else {
			if len(blockDevices) > 0 {
				fmt.Printf("\nblock devices have total size %s\n", humanize.Bytes(blockDevices.GetTotalBytes()))
			}
			blockDevices.RenderTable(os.Stdout)
			println()
		}

		infos, err := fuse.ListConnections()
		if err != nil {
			log.Logger.Warnw("error listing fuse connections", "error", err)
		} else {
			fmt.Printf("%s listed %d fuse connections\n", checkMark, len(infos))
			infos.RenderTable(os.Stdout)
			println()
		}
	}

	fmt.Printf("\n\n%s scan complete\n\n", checkMark)
	return nil
}

func scanKmsg(ctx context.Context) (int, error) {
	messages, err := kmsg.ReadAll(ctx)
	if err != nil {
		fmt.Printf("%s failed to read kmsg: %v\n", warningSign, err)
		return 0, err
	}

	if len(messages) == 0 {
		return 0, nil
	}

	issueCount := 0
	ts := messages[0].DescribeTimestamp(time.Now().UTC())
	fmt.Printf("%s first kmsg line is %s old\n", checkMark, ts)

	for _, msg := range messages {
		if time.Since(msg.Timestamp.Time) > eventstore.DefaultRetention {
			continue
		}

		ts = msg.DescribeTimestamp(time.Now().UTC())

		if ev, m := cpucomponent.Match(msg.Message); m != "" {
			fmt.Printf("[CPU] (%s) %s %s %q\n", ts, ev, m, msg.Message)
			issueCount++
		} else if ev, m := memorycomponent.Match(msg.Message); m != "" {
			fmt.Printf("[Memory] (%s) %s %s %q\n", ts, ev, m, msg.Message)
			issueCount++
		} else if ev, m := fd.Match(msg.Message); m != "" {
			fmt.Printf("[File Descriptor] (%s) %s %s %q\n", ts, ev, m, msg.Message)
			issueCount++
		} else if found := nvidia_xid.Match(msg.Message); found != nil {
			fmt.Printf("[NVIDIA XID] (%s) %q\n", ts, msg.Message)
			issueCount++
		} else if found := nvidia_sxid.Match(msg.Message); found != nil {
			fmt.Printf("[NVIDIA XID] (%s) %q\n", ts, msg.Message)
			issueCount++
		} else if ev, m := nvidia_nccl.Match(msg.Message); m != "" {
			fmt.Printf("[NVIDIA NCCL] (%s) %s %s %q\n", ts, ev, m, msg.Message)
			issueCount++
		} else if ev, m := nvidia_peermem.Match(msg.Message); m != "" {
			fmt.Printf("[NVIDIA Peermem] (%s) %s %s %q\n", ts, ev, m, msg.Message)
			issueCount++
		}
	}
	return issueCount, nil
}
