package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_clock_events_state "github.com/leptonai/gpud/components/accelerator/nvidia/query/clock-events-state"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/components/dmesg"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"
	query_log_tail "github.com/leptonai/gpud/components/query/log/tail"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/fuse"
	"github.com/leptonai/gpud/pkg/host"
	latency_edge "github.com/leptonai/gpud/pkg/latency/edge"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/sqlite"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	inProgress  = "\033[33m⌛\033[0m"
	checkMark   = "\033[32m✔\033[0m"
	warningSign = "\033[31m✘\033[0m"
)

var defaultNVIDIALibraries = []string{
	"libnvidia-ml.so",
	"libcuda.so",
}

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

		for _, lib := range defaultNVIDIALibraries {
			libPath, err := file.FindLibrary(lib, file.WithSearchDirs(
				// ref. https://github.com/NVIDIA/nvidia-container-toolkit/blob/main/internal/lookup/library.go#L33-L62
				"/",
				"/usr/lib64",
				"/usr/lib/x86_64-linux-gnu",
				"/usr/lib/aarch64-linux-gnu",
				"/usr/lib/x86_64-linux-gnu/nvidia/current",
				"/usr/lib/aarch64-linux-gnu/nvidia/current",
				"/lib64",
				"/lib/x86_64-linux-gnu",
				"/lib/aarch64-linux-gnu",
				"/lib/x86_64-linux-gnu/nvidia/current",
				"/lib/aarch64-linux-gnu/nvidia/current",
			))
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

		// "nvidia_query.Get" assumes that the "clock-events-state" table exists
		// pre-create since this is a one-off operation
		// TODO: move these into a single place
		if err := nvidia_clock_events_state.CreateTable(ctx, db); err != nil {
			log.Logger.Fatalw("failed to create clock events state table", "error", err)
		}

		outputRaw, err := nvidia_query.Get(
			ctx,
			nvidia_query.WithDBRW(db),
			nvidia_query.WithDBRO(db),
			nvidia_query.WithNvidiaSMICommand(op.nvidiaSMICommand),
			nvidia_query.WithNvidiaSMIQueryCommand(op.nvidiaSMIQueryCommand),
			nvidia_query.WithIbstatCommand(op.ibstatCommand),
			nvidia_query.WithInfinibandClassDirectory(op.infinibandClassDirectory),
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
				output.PrintInfo(query.WithDebug(op.debug), query.WithInfinibandClassDirectory(op.infinibandClassDirectory))

				if op.pollXidEvents {
					fmt.Printf("\n%s checking nvidia xid errors\n", inProgress)

					select {
					case <-ctx.Done():
						log.Logger.Warnw("context done")

					case <-time.After(7 * time.Second):
						fmt.Printf("%s no xid events found after 7 seconds\n", checkMark)

					case event := <-nvidia_query_nvml.DefaultInstance().RecvXidEvents():
						if event.Error != nil {
							fmt.Printf("%s received the xid event with an error %v\n", checkMark, event.Error)
						} else {
							if nvidia_query_nvml.DefaultInstance().XidErrorSupported() {
								fmt.Printf("%s successfully received the xid event with no error\n", warningSign)
							} else {
								fmt.Printf("%s xid error not supported\n", warningSign)
							}
						}

						yb, _ := event.YAML()
						fmt.Println(string(yb))
						println()
					}
				}

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
			}
		}
	}
	println()

	if op.dmesgCheck {
		if os.Geteuid() != 0 {
			return errors.New("requires sudo/root access in order to scan dmesg errors")
		}

		fmt.Printf("%s scanning dmesg for %d lines\n", inProgress, op.lines)
		defaultDmesgCfg, err := dmesg.DefaultConfig(ctx)
		if err != nil {
			return err
		}
		matched, err := query_log_tail.Scan(
			ctx,
			query_log_tail.WithDedup(true),
			query_log_tail.WithCommands(defaultDmesgCfg.Log.Scan.Commands),
			query_log_tail.WithLinesToTail(op.lines),
			query_log_tail.WithSelectFilter(defaultDmesgCfg.Log.SelectFilters...),
			query_log_tail.WithExtractTime(defaultDmesgCfg.Log.TimeParseFunc),
			query_log_tail.WithProcessMatched(func(time time.Time, line []byte, matched *query_log_common.Filter) {
				log.Logger.Debugw("matched", "line", string(line))
				fmt.Println("line", string(line))
				matchedB, _ := matched.YAML()
				fmt.Println(string(matchedB))

				if xid := nvidia_query_xid.ExtractNVRMXid(string(line)); xid > 0 {
					if dm, err := nvidia_query_xid.ParseDmesgLogLine(metav1.Time{Time: time}, string(line)); err == nil {
						log.Logger.Warnw("known xid", "line", string(line))
						yb, _ := dm.YAML()
						fmt.Println(string(yb))
					}
				}

				if sxid := nvidia_query_sxid.ExtractNVSwitchSXid(string(line)); sxid > 0 {
					if dm, err := nvidia_query_sxid.ParseDmesgLogLine(metav1.Time{Time: time}, string(line)); err == nil {
						log.Logger.Warnw("known sxid", "line", string(line))
						yb, _ := dm.YAML()
						fmt.Println(string(yb))
					}
				}
			}),
		)
		if err != nil {
			return err
		}
		if matched == 0 {
			fmt.Printf("%s scanned dmesg file -- found no issue\n", checkMark)
		} else {
			fmt.Printf("%s scanned dmesg file -- found %d issue(s)\n", warningSign, matched)
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
