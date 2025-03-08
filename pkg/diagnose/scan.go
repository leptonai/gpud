package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	nvidia_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid"
	nvidia_xid "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	nvidia_nccl "github.com/leptonai/gpud/components/accelerator/nvidia/nccl"
	nvidia_peermem "github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/fd"
	"github.com/leptonai/gpud/components/memory"
	"github.com/leptonai/gpud/pkg/disk"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/fuse"
	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	latency_edge "github.com/leptonai/gpud/pkg/netutil/latency/edge"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/sqlite"

	"github.com/dustin/go-humanize"
)

const (
	inProgress  = "\033[33m⌛\033[0m"
	checkMark   = "\033[32m✔\033[0m"
	warningSign = "\033[31m✘\033[0m"
)

// Runs the scan operations.
func Scan(ctx context.Context, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	fmt.Printf("\n\n%s scanning the host (GOOS %s)\n\n", inProgress, runtime.GOOS)
	machineID, err := host.GetMachineID(ctx)
	if err != nil {
		log.Logger.Warnw("error reading machine ID", "error", err)
	} else {
		fmt.Printf("%s detected machine ID %q\n", checkMark, machineID)
	}

	bootID, err := host.GetBootID()
	if err != nil {
		log.Logger.Warnw("error reading boot ID", "error", err)
	} else {
		fmt.Printf("%s detected boot ID %q\n", checkMark, bootID)
	}

	virtEnv, err := host.SystemdDetectVirt(ctx)
	if err != nil {
		log.Logger.Warnw("error detecting virtualization environment", "error", err)
	} else {
		fmt.Printf("%s detected virtualization environment %q\n", checkMark, virtEnv.Type)
	}
	manufacturer, err := host.SystemManufacturer(ctx)
	if err != nil {
		log.Logger.Warnw("error detecting system manufacturer", "error", err)
	} else {
		fmt.Printf("%s detected system manufacturer %q\n", checkMark, manufacturer)
	}

	fmt.Printf("%s scanning the process counts\n", inProgress)
	processCountsByStatus, err := process.CountProcessesByStatus(ctx)
	if err != nil {
		log.Logger.Warnw("error counting processes by status", "error", err)
	} else {
		fmt.Printf("%s counted processes by status\n", checkMark)
		for status, count := range processCountsByStatus {
			fmt.Printf("%s %q %d\n", checkMark, status, len(count))
		}
	}

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		log.Logger.Warnw("error checking nvidia gpu installation", "error", err)
		return err
	}

	if nvidiaInstalled {
		fmt.Printf("\n%s scanning nvidia accelerators\n", inProgress)

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

		db, err := sqlite.Open(":memory:")
		if err != nil {
			log.Logger.Fatalw("failed to open database", "error", err)
		}
		defer db.Close()

		eventStore, err := eventstore.New(db, db)
		if err != nil {
			log.Logger.Fatalw("failed to open database", "error", err)
		}

		xidEventBucket, err := eventStore.Bucket(nvidia_component_error_xid_id.Name, 0)
		if err != nil {
			log.Logger.Fatalw("failed to create events bucket", "error", err)
		}

		hwSlowdownEventBucket, err := eventStore.Bucket(nvidia_hw_slowdown_id.Name, 0)
		if err != nil {
			log.Logger.Fatalw("failed to create events bucket", "error", err)
		}

		outputRaw, err := nvidia_query.Get(
			ctx,
			nvidia_query.WithXidEventBucket(xidEventBucket),
			nvidia_query.WithHWSlowdownEventBucket(hwSlowdownEventBucket),
			nvidia_query.WithNvidiaSMIQueryCommand(op.nvidiaSMIQueryCommand),
			nvidia_query.WithIbstatCommand(op.ibstatCommand),
		)
		if err != nil {
			log.Logger.Warnw("error getting nvidia info", "error", err)
		} else {
			defer func() {
				serr := nvidia_query_nvml.DefaultInstance().Shutdown()
				if serr != nil {
					log.Logger.Warnw("error shutting down NVML", "error", serr)
				}
			}()

			output, ok := outputRaw.(*nvidia_query.Output)
			if !ok {
				log.Logger.Warnf("expected *nvidia_query.Output, got %T", outputRaw)
			} else {
				output.PrintInfo(nvidia_query.WithDebug(op.debug))

				if op.pollGPMEvents {
					fmt.Printf("\n%s checking nvidia GPM events\n", inProgress)

					gpmSupported, err := nvidia_query_nvml.GPMSupported()
					if err == nil {
						if gpmSupported {
							log.Logger.Infow("auto-detected gpm supported")
						} else {
							log.Logger.Infow("auto-detected gpm not supported -- skipping", "error", err)
						}
					} else {
						log.Logger.Warnw("failed to check gpm supported or not", "error", err)
					}

					if gpmSupported {
						select {
						case <-ctx.Done():
							log.Logger.Warnw("context done")

						case <-time.After(time.Minute + 10*time.Second):
							fmt.Printf("%s no gpm events found after 70 seconds\n", checkMark)

						case event := <-nvidia_query_nvml.DefaultInstance().RecvGPMEvents():
							if event != nil && event.Error != nil {
								fmt.Printf("%s received the gpm event with an error %v\n", checkMark, event.Error)
							} else {
								if nvidia_query_nvml.DefaultInstance().GPMMetricsSupported() {
									fmt.Printf("%s successfully received the gpm event with no error\n", checkMark)
								} else {
									fmt.Printf("%s gpm metrics not supported\n", checkMark)
								}
							}

							yb, _ := event.YAML()
							fmt.Println(string(yb))
							println()
						}
					}
				}

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
		}
	}
	println()

	if op.dmesgCheck {
		if os.Geteuid() != 0 {
			return errors.New("requires sudo/root access in order to scan dmesg errors")
		}

		fmt.Printf("%s scanning dmesg\n", inProgress)
		issueCnt, err := scanDmesg(ctx)
		if err != nil {
			return err
		}
		if issueCnt == 0 {
			fmt.Printf("%s scanned dmesg file -- found no issue\n", checkMark)
		} else {
			fmt.Printf("%s scanned dmesg file -- found %d issue(s)\n", warningSign, issueCnt)
		}

		scanKmsg(ctx)
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

func scanDmesg(ctx context.Context) (int, error) {
	p, err := process.New(
		process.WithCommands(pkg_dmesg.DefaultDmesgScanCommands),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return 0, err
	}
	if err := p.Start(ctx); err != nil {
		return 0, err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	nowUTC := time.Now().UTC()
	issueCnt := 0
	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			parsed := pkg_dmesg.ParseDmesgLine(line)
			ts := humanize.RelTime(parsed.Timestamp, nowUTC, "ago", "from now")

			if found := nvidia_xid.Match(line); found != nil {
				fmt.Printf("[XID found] (%s) %q\n", ts, parsed.Content)
				issueCnt++
				return
			}

			if found := nvidia_sxid.Match(line); found != nil {
				fmt.Printf("[SXID found] (%s) %q\n", ts, parsed.Content)
				issueCnt++
				return
			}
		}),
	); err != nil {
		return 0, err
	}

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case err := <-p.Wait():
		if err != nil {
			return 0, err
		}
	}

	return issueCnt, nil
}

func scanKmsg(ctx context.Context) {
	if os.Geteuid() != 0 {
		fmt.Printf("%s skipping kmsg scan (requires sudo/root access)\n", checkMark)
		return
	}

	fmt.Printf("%s scanning kmsg\n", inProgress)
	msgs, err := kmsg.ReadAll(ctx)
	if err != nil {
		fmt.Printf("%s failed to read kmsg: %v\n", warningSign, err)
		return
	}

	fmt.Printf("%s scanned kmsg file -- found %d line(s)\n", checkMark, len(msgs))
	if len(msgs) == 0 {
		return
	}

	ts := msgs[0].DescribeTimestamp(time.Now().UTC())
	fmt.Printf("%s first kmsg line is %s old\n", checkMark, ts)

	for _, msg := range msgs {
		ts = msg.DescribeTimestamp(time.Now().UTC())

		if ev, m := cpu.Match(msg.Message); m != "" {
			fmt.Printf("[cpu] (%s) %s %s %q\n", ts, ev, m, msg.Message)
		}
		if ev, m := memory.Match(msg.Message); m != "" {
			fmt.Printf("[memory] (%s) %s %s %q\n", ts, ev, m, msg.Message)
		}
		if ev, m := fd.Match(msg.Message); m != "" {
			fmt.Printf("[file descriptor] (%s) %s %s %q\n", ts, ev, m, msg.Message)
		}
		if found := nvidia_xid.Match(msg.Message); found != nil {
			fmt.Printf("[nvidia xid] (%s) %q\n", ts, msg.Message)
		}
		if found := nvidia_sxid.Match(msg.Message); found != nil {
			fmt.Printf("[nvidia sxid] (%s) %q\n", ts, msg.Message)
		}
		if ev, m := nvidia_nccl.Match(msg.Message); m != "" {
			fmt.Printf("[nvidia nccl] (%s) %s %s %q\n", ts, ev, m, msg.Message)
		}
		if ev, m := nvidia_peermem.Match(msg.Message); m != "" {
			fmt.Printf("[nvidia peermem] (%s) %s %s %q\n", ts, ev, m, msg.Message)
		}
	}
}
