package dmesg

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/process"
	poller_config "github.com/leptonai/gpud/poller/config"
	poller_log_config "github.com/leptonai/gpud/poller/log/config"
)

type Config struct {
	Log poller_log_config.Config `json:"log"`
}

func ParseConfig(b any, dbRW *sql.DB, dbRO *sql.DB) (*Config, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	cfg := new(Config)
	err = json.Unmarshal(raw, cfg)
	if err != nil {
		return nil, err
	}

	if cfg.Log.PollerConfig.State != nil {
		cfg.Log.PollerConfig.State.DBRW = dbRW
		cfg.Log.PollerConfig.State.DBRO = dbRO
	}

	return cfg, nil
}

func (cfg Config) Validate() error {
	return cfg.Log.Validate()
}

func DmesgExists() bool {
	p, err := exec.LookPath("dmesg")
	if err != nil {
		return false
	}
	return p != ""
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

func DefaultConfig(ctx context.Context) (Config, error) {
	// isUse is false ==> journalctl, true ==> dmesg
	if supported := checkDmesgSupportsSinceFlag(ctx); !supported {
		return journalCtlDefaultConfig(ctx)
	}

	defaultFilters, err := DefaultLogFilters(ctx)
	if err != nil {
		return Config{}, err
	}

	scanCommands := [][]string{{DefaultScanDmesgCmd}}
	if _, err := os.Stat(DefaultDmesgFile); os.IsNotExist(err) {
		scanCommands = [][]string{{DefaultScanDmesgCmd}}
	}

	cfg := Config{
		Log: poller_log_config.Config{
			PollerConfig: poller_config.DefaultConfig(),
			BufferSize:   poller_log_config.DefaultBufferSize,

			Commands: [][]string{
				// run last commands as fallback, in case dmesg flag only works in some machines
				{DefaultDmesgCmdWithSince + " -w || " + DefaultDmesgCmd + " -w || true"},
				{DefaultDmesgCmdWithSince + " -W || " + DefaultDmesgCmd + " -W"},
			},

			Scan: &poller_log_config.Scan{
				Commands:    scanCommands,
				LinesToTail: 10000,
			},

			SelectFilters: defaultFilters,

			TimeParseFunc: dmesg.ParseISOtimeWithError,
		},
	}
	return cfg, nil
}

// journalCtlDefaultConfig In older util-linux version it can`t compatible dmesg command args
// like --time-format or --since. So, we will use journalctl -k to instead it.
func journalCtlDefaultConfig(ctx context.Context) (Config, error) {
	defaultFilters, err := DefaultLogFilters(ctx)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Log: poller_log_config.Config{
			PollerConfig: poller_config.DefaultConfig(),
			BufferSize:   poller_log_config.DefaultBufferSize,

			Commands: [][]string{{DefaultJournalCtlScanCmd}},

			Scan: &poller_log_config.Scan{
				Commands:    [][]string{{DefaultJournalCtlCmd}},
				LinesToTail: 10000,
			},

			SelectFilters: defaultFilters,

			TimeParseFunc: dmesg.ParseShortISOtimeWithError,
		},
	}
	return cfg, nil
}
