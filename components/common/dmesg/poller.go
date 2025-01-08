package dmesg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	nvidia_component_error_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid/id"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_query_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/query/sxid"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	nvidia_xid_sxid_state "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid-sxid-state"
	query_config "github.com/leptonai/gpud/components/query/config"
	query_log "github.com/leptonai/gpud/components/query/log"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"
	query_log_config "github.com/leptonai/gpud/components/query/log/config"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/dmesg"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/process"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Name of the common dmesg poller.
const Name = "dmesg"

var (
	defaultLogPollerOnce sync.Once
	defaultLogPoller     query_log.Poller
)

// only set once since it relies on the kube client and specific port
func SetDefaultLogPoller(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("os %q not supported for dmesg log poller", runtime.GOOS)
	}

	asRoot := os.Geteuid() == 0 // running as root
	if !asRoot {
		return errors.New("dmesg log poller requires root privileges")
	}

	if !dmesgExists() {
		return errors.New("dmesg not found")
	}

	logFilters, err := getDefaultLogFilters(ctx)
	if err != nil {
		return err
	}
	logConfig := getDefaultLogConfig(ctx, logFilters)

	defaultLogPollerOnce.Do(func() {
		defaultLogPoller, err = query_log.New(
			ctx,
			logConfig,
			pkg_dmesg.ParseISOtimeWithError,
			createProcessMatched(ctx, dbRW, dbRO),
		)
		if err != nil {
			panic(err)
		}
	})
	return err
}

func GetDefaultLogPoller() query_log.Poller {
	return defaultLogPoller
}

func createProcessMatched(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB) query_log_common.ProcessMatchedFunc {
	return func(ts time.Time, line []byte, matchedFilter *query_log_common.Filter) {
		if ts.IsZero() {
			return
		}
		if len(line) == 0 {
			return
		}
		if matchedFilter == nil {
			return
		}

		cctx, ccancel := context.WithTimeout(ctx, 10*time.Second)
		defer ccancel()

		for _, ref := range matchedFilter.OwnerReferences {
			switch ref {
			case nvidia_component_error_xid_id.Name:
				ev, err := nvidia_query_xid.ParseDmesgLogLine(metav1.Time{Time: ts}, string(line))
				if err != nil {
					log.Logger.Errorw("failed to parse xid dmesg line", "line", string(line), "error", err)
					continue
				}
				if ev.Detail == nil {
					log.Logger.Errorw("failed to parse xid dmesg line", "line", string(line), "error", "no detail")
					continue
				}

				eventToInsert := nvidia_xid_sxid_state.Event{
					UnixSeconds:  ts.Unix(),
					DataSource:   "dmesg",
					EventType:    "xid",
					EventID:      int64(ev.Detail.Xid),
					DeviceID:     ev.DeviceUUID,
					EventDetails: ev.LogItem.Line,
				}

				found, err := nvidia_xid_sxid_state.FindEvent(cctx, dbRO, eventToInsert)
				if err != nil {
					log.Logger.Errorw("failed to find xid event in database", "error", err)
					continue
				}
				if found {
					log.Logger.Debugw("xid event already exists in database", "event", eventToInsert)
					continue
				}
				if werr := nvidia_xid_sxid_state.InsertEvent(cctx, dbRW, eventToInsert); werr != nil {
					log.Logger.Errorw("failed to insert xid event into database", "error", werr)
					continue
				}

			case nvidia_component_error_sxid_id.Name:
				ev, err := nvidia_query_sxid.ParseDmesgLogLine(metav1.Time{Time: ts}, string(line))
				if err != nil {
					log.Logger.Errorw("failed to parse sxid dmesg line", "line", string(line), "error", err)
					continue
				}
				if ev.Detail == nil {
					log.Logger.Errorw("failed to parse sxid dmesg line", "line", string(line), "error", "no detail")
					continue
				}

				eventToInsert := nvidia_xid_sxid_state.Event{
					UnixSeconds:  ts.Unix(),
					DataSource:   "dmesg",
					EventType:    "sxid",
					EventID:      int64(ev.Detail.SXid),
					DeviceID:     ev.DeviceUUID,
					EventDetails: ev.LogItem.Line,
				}

				found, err := nvidia_xid_sxid_state.FindEvent(cctx, dbRO, eventToInsert)
				if err != nil {
					log.Logger.Errorw("failed to find sxid event in database", "error", err)
					continue
				}
				if found {
					log.Logger.Debugw("sxid event already exists in database", "event", eventToInsert)
					continue
				}
				if werr := nvidia_xid_sxid_state.InsertEvent(cctx, dbRW, eventToInsert); werr != nil {
					log.Logger.Errorw("failed to insert sxid event into database", "error", werr)
					continue
				}
			}
		}
	}
}

func getDefaultLogFilters(ctx context.Context) ([]*query_log_common.Filter, error) {
	defaultFilters := DefaultDmesgFiltersForMemory()
	defaultFilters = append(defaultFilters, DefaultDmesgFiltersForCPU()...)
	defaultFilters = append(defaultFilters, DefaultDmesgFiltersForFileDescriptor()...)

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if err != nil {
		return nil, err
	}
	if nvidiaInstalled {
		defaultFilters = append(defaultFilters, DefaultDmesgFiltersForNvidia()...)
	}

	for i := range defaultFilters {
		if err := defaultFilters[i].Compile(); err != nil {
			return nil, err
		}
	}
	return defaultFilters, nil
}

func getDefaultLogConfig(ctx context.Context, logFilters []*query_log_common.Filter) query_log_config.Config {
	if supported := checkDmesgSupportsSinceFlag(ctx); !supported {
		// dmesg --time-format or --since not supported
		// thus fallback to journalctl
		return query_log_config.Config{
			Query:      query_config.DefaultConfig(),
			BufferSize: query_log_config.DefaultBufferSize,

			Commands: [][]string{{DefaultJournalCtlScanCmd}},

			Scan: &query_log_config.Scan{
				Commands:    [][]string{{DefaultJournalCtlCmd}},
				LinesToTail: 10000,
			},

			SelectFilters: logFilters,

			TimeParseFunc: dmesg.ParseShortISOtimeWithError,
		}
	}

	scanCommands := [][]string{{DefaultScanDmesgCmd}}
	if _, err := os.Stat(DefaultDmesgFile); os.IsNotExist(err) {
		scanCommands = [][]string{{DefaultScanDmesgCmd}}
	}

	return query_log_config.Config{
		Query:      query_config.DefaultConfig(),
		BufferSize: query_log_config.DefaultBufferSize,

		Commands: [][]string{
			// run last commands as fallback, in case dmesg flag only works in some machines
			{DefaultDmesgCmdWithSince + " -w || " + DefaultDmesgCmd + " -w || true"},
			{DefaultDmesgCmdWithSince + " -W || " + DefaultDmesgCmd + " -W"},
		},

		Scan: &query_log_config.Scan{
			Commands:    scanCommands,
			LinesToTail: 10000,
		},

		SelectFilters: logFilters,

		TimeParseFunc: pkg_dmesg.ParseISOtimeWithError,
	}
}

const (
	// DefaultDmesgFile default path with dmesg file
	DefaultDmesgFile = "/var/log/dmesg"
	// DefaultDmesgCmd DefaultDmesgCmdWithSince default scan dmesg command (in newer util-linux it works, but older is not)
	// some old dmesg versions don't support --since, thus fall back to the one without --since and tail the last 200 lines
	// ref. https://github.com/leptonai/gpud/issues/32
	DefaultDmesgCmd          = "dmesg --time-format=iso --nopager --buffer-size 163920"
	DefaultDmesgCmdWithSince = "dmesg --time-format=iso --nopager --buffer-size 163920 --since '1 hour ago'"
	DefaultScanDmesgCmd      = DefaultDmesgCmdWithSince + " || " + DefaultDmesgCmd + " | tail -n 200"

	// dmesg --version that supports "--since" flag
	// ref. https://github.com/util-linux/util-linux/blob/master/Documentation/releases/v2.37-ReleaseNotes
	dmesgSinceFlagSupportVersion = 2.37

	// DefaultJournalCtlCmd default scan journalctl command
	DefaultJournalCtlCmd     = "journalctl -qk -o short-iso --no-pager --since '1 hour ago' | tail -n 200"
	DefaultJournalCtlScanCmd = "journalctl -qk -o short-iso --no-pager --since '1 hour ago' -f || true"
)

var dmesgVersionRegPattern = regexp.MustCompile(`\d+\.\d+`)

func checkDmesgVersionOutputForSinceFlag(verOutput string) bool {
	matches := dmesgVersionRegPattern.FindString(verOutput)
	if matches != "" {
		if versionF, parseErr := strconv.ParseFloat(matches, 64); parseErr == nil {
			if versionF >= dmesgSinceFlagSupportVersion {
				return true
			}
		}
	}

	return false
}

func checkDmesgSupportsSinceFlag(ctx context.Context) bool {
	p, err := process.New(
		process.WithCommand("dmesg --version"),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return false
	}

	if err := p.Start(ctx); err != nil {
		return false
	}

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
		return false
	}

	line := strings.Join(lines, "\n")
	line = strings.TrimSpace(line)

	return checkDmesgVersionOutputForSinceFlag(line)
}

func dmesgExists() bool {
	p, err := exec.LookPath("dmesg")
	if err != nil {
		return false
	}
	return p != ""
}
