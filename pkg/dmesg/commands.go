package dmesg

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/process"
)

type Commands struct {
	WatchCommands [][]string
	TailCommands  [][]string
	ParseTimeFunc func(line []byte) (time.Time, []byte, error)
}

func GetCommands(ctx context.Context) (*Commands, error) {
	sinceSupported, err := checkDmesgSupportsSinceFlag(ctx)
	if err != nil {
		return nil, err
	}

	if sinceSupported {
		return &Commands{
			WatchCommands: WatchCommandsDmesg(),
			TailCommands:  TailCommandsDmesg(),
			ParseTimeFunc: ParseTimeFuncDmesg(),
		}, nil
	}

	return &Commands{
		WatchCommands: WatchCommandsJournalctl(),
		TailCommands:  TailCommandsJournalctl(),
		ParseTimeFunc: ParseTimeFuncJournalctl(),
	}, nil
}

const (
	// defaultDmesgCmd DefaultDmesgCmdWithSince default scan dmesg command (in newer util-linux it works, but older is not)
	// some old dmesg versions don't support --since, thus fall back to the one without --since and tail the last 200 lines
	// ref. https://github.com/leptonai/gpud/issues/32
	defaultDmesgCmd          = "dmesg --time-format=iso --nopager --buffer-size 163920"
	defaultDmesgCmdWithSince = "dmesg --time-format=iso --nopager --buffer-size 163920 --since '1 hour ago'"
	defaultDmesgTailCmd      = defaultDmesgCmdWithSince + " || " + defaultDmesgCmd + " | tail -n 200"
)

func WatchCommandsDmesg() [][]string {
	return [][]string{
		{defaultDmesgCmdWithSince + " -w || " + defaultDmesgCmd + " -w || true"},

		// run last commands as fallback, in case "dmesg -w" flag only works in some machines
		{defaultDmesgCmdWithSince + " -W || " + defaultDmesgCmd + " -W"},
	}
}

func TailCommandsDmesg() [][]string {
	return [][]string{
		{defaultDmesgTailCmd},
	}
}

func ParseTimeFuncDmesg() func(line []byte) (time.Time, []byte, error) {
	return ParseDmesgTimeISO
}

const (
	// DefaultJournalCtlCmd default scan journalctl command
	// in case "dmesg --time-format" or "dmesg --since" flags are not supported
	defaultJournalCtlWatchCmd = "journalctl -qk -o short-iso --no-pager --since '1 hour ago' -f || true"
	defaultJournalCtlTailCmd  = "journalctl -qk -o short-iso --no-pager --since '1 hour ago' | tail -n 200"
)

func WatchCommandsJournalctl() [][]string {
	return [][]string{
		{defaultJournalCtlWatchCmd},
	}
}

func TailCommandsJournalctl() [][]string {
	return [][]string{
		{defaultJournalCtlTailCmd},
	}
}

func ParseTimeFuncJournalctl() func(line []byte) (time.Time, []byte, error) {
	return ParseJournalctlTimeShortISO
}

// Returns true if dmesg supports "dmesg --since" flag, otherwise returns false.
func checkDmesgSupportsSinceFlag(ctx context.Context) (bool, error) {
	if !process.CommandExists("dmesg") {
		return false, nil
	}

	p, err := process.New(
		process.WithCommand("dmesg --version"),
		process.WithRunAsBashScript(),
	)
	if err != nil {
		return false, err
	}

	if err := p.Start(ctx); err != nil {
		return false, err
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
		return false, err
	}

	line := strings.Join(lines, "\n")
	line = strings.TrimSpace(line)

	return checkDmesgVersionOutputForSinceFlag(line), nil
}

// dmesg --version that supports "--since" flag
// ref. https://github.com/util-linux/util-linux/blob/master/Documentation/releases/v2.37-ReleaseNotes
const dmesgSinceFlagSupportVersion = 2.37

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
