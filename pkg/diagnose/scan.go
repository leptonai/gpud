package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"time"

	nvidia_component_error_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid"
	nvidia_component_error_xid "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	"github.com/leptonai/gpud/pkg/disk"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/fuse"
	"github.com/leptonai/gpud/pkg/host"
	latency_edge "github.com/leptonai/gpud/pkg/latency/edge"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	nvidia_query_nvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/process"
	query_log_common "github.com/leptonai/gpud/pkg/query/log/common"
	query_log_tail "github.com/leptonai/gpud/pkg/query/log/tail"
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

		eventsStoreNvidiaErrorXid, err := events_db.NewStore(
			db,
			db,
			events_db.CreateDefaultTableName(nvidia_component_error_xid_id.Name),
			3*24*time.Hour,
		)
		if err != nil {
			log.Logger.Fatalw("failed to create events store", "error", err)
		}

		eventsStoreNvidiaHWSlowdown, err := events_db.NewStore(
			db,
			db,
			events_db.CreateDefaultTableName(nvidia_hw_slowdown_id.Name),
			3*24*time.Hour,
		)
		if err != nil {
			log.Logger.Fatalw("failed to create events store", "error", err)
		}

		outputRaw, err := nvidia_query.Get(
			ctx,
			nvidia_query.WithDBRW(db), // to deprecate in favor of events store
			nvidia_query.WithDBRO(db), // to deprecate in favor of events store
			nvidia_query.WithXidEventsStore(eventsStoreNvidiaErrorXid),
			nvidia_query.WithHWSlowdownEventsStore(eventsStoreNvidiaHWSlowdown),
			nvidia_query.WithNvidiaSMICommand(op.nvidiaSMICommand),
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

		fmt.Printf("%s scanning dmesg for %d lines\n", inProgress, op.lines)
		matched, err := query_log_tail.Scan(
			ctx,
			query_log_tail.WithDedup(true),
			query_log_tail.WithCommands(pkg_dmesg.DefaultDmesgScanCommands),
			query_log_tail.WithLinesToTail(op.lines),
			query_log_tail.WithMatchFunc(
				func(line string) (string, string) {
					xidErr := nvidia_component_error_xid.Match(line)
					if xidErr != nil {
						return "xid found", ""
					}
					return "", "" // no match
				},
				func(line string) (string, string) {
					sxidErr := nvidia_component_error_sxid.Match(line)
					if sxidErr != nil {
						return "sxid found", ""
					}
					return "", "" // no match
				},
			),
			query_log_tail.WithExtractTime(func(l []byte) (time.Time, []byte, error) {
				dm := pkg_dmesg.ParseDmesgLine(string(l))
				return dm.Timestamp, l, nil
			}),
			query_log_tail.WithProcessMatched(func(time time.Time, line []byte, matched *query_log_common.Filter) {
				if xidErr := nvidia_component_error_xid.Match(string(line)); xidErr != nil {
					log.Logger.Warnw("known xid", "line", string(line))
					yb, _ := xidErr.YAML()
					fmt.Println(string(yb))
				}

				if sxidErr := nvidia_component_error_sxid.Match(string(line)); sxidErr != nil {
					log.Logger.Warnw("known sxid", "line", string(line))
					yb, _ := sxidErr.YAML()
					fmt.Println(string(yb))
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
