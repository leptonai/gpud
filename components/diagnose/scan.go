package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_nvml "github.com/leptonai/gpud/components/accelerator/nvidia/query/nvml"
	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/components/dmesg"
	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"
	query_log_tail "github.com/leptonai/gpud/components/query/log/tail"
	"github.com/leptonai/gpud/log"
)

const (
	inProgress  = "\033[33m⌛\033[0m"
	checkMark   = "\033[32m✔\033[0m"
	warningSign = "\033[31m✘\033[0m"
)

// Runs the scan operations.
func Scan(ctx context.Context, opts ...OpOption) error {
	if os.Geteuid() != 0 {
		return errors.New("requires sudo/root access in order to scan dmesg errors")
	}

	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	fmt.Printf("\n\n%s scanning the host\n\n", inProgress)

	if nvidia_query.SMIExists() {
		fmt.Printf("%s scanning nvidia accelerators\n", inProgress)

		outputRaw, err := nvidia_query.Get(ctx)
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
				output.PrintInfo(op.debug)

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

	fmt.Printf("%s scanning dmesg for %d lines\n", inProgress, op.lines)
	defaultDmesgCfg := dmesg.DefaultConfig()
	matched, err := query_log_tail.Scan(
		ctx,
		query_log_tail.WithCommands(defaultDmesgCfg.Log.Scan.Commands),
		query_log_tail.WithLinesToTail(op.lines),
		query_log_tail.WithSelectFilter(defaultDmesgCfg.Log.SelectFilters...),
		query_log_tail.WithParseTime(dmesg.ExtractTimeFromLogLine),
		query_log_tail.WithProcessMatched(func(line []byte, time time.Time, matched *query_log_filter.Filter) {
			log.Logger.Debugw("matched", "line", string(line))
			matchedB, _ := matched.YAML()
			fmt.Println(string(matchedB))

			if xid := nvidia_query_xid.ExtractNVRMXid(string(line)); xid > 0 {
				if dm, err := nvidia_query_xid.ParseDmesgLogLine(string(line)); err == nil {
					log.Logger.Warnw("known xid", "line", string(line))
					yb, _ := dm.YAML()
					fmt.Println(string(yb))
				}
			}

			if sxid := nvidia_query_sxid.ExtractNVSwitchSXid(string(line)); sxid > 0 {
				if dm, err := nvidia_query_sxid.ParseDmesgLogLine(string(line)); err == nil {
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

	fmt.Printf("\n\n%s scan complete\n\n", checkMark)
	return nil
}
