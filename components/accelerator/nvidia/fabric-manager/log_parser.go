package fabricmanager

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/log/streamer"
)

const fabricmanagerLogTimeFormat = "Jan 02 2006 15:04:05"

var (
	fabricmanagerLogTimeFormatN = len(fabricmanagerLogTimeFormat) + 2 // [ ]
	regexForFabricmanagerLog    = regexp.MustCompile(`^\[([^\]]+)\]`)
)

// ParseFabricManagerLog parses a fabric manager log line and returns a streamer.LogLine.
// It implements the streamer.ParseLogFunc type.
// The fabric manager log format is:
// [Feb 25 2025 13:59:45] [INFO] [tid 1803] Received an inband message
func ParseFabricManagerLog(line string) streamer.LogLine {
	logLine := streamer.LogLine{Time: time.Now().UTC(), Content: line}

	matches := regexForFabricmanagerLog.FindStringSubmatch(line)
	if len(matches) == 0 {
		log.Logger.Warnw("no timestamp matches found", "line", line)
		logLine.Error = errors.New("no timestamp matches found")
		return logLine
	}

	s := matches[1]
	parsedTime, err := time.Parse(fabricmanagerLogTimeFormat, s)
	if err != nil {
		log.Logger.Warnw("failed to parse timestamp", "line", line, "error", err)
		logLine.Error = err
		return logLine
	}
	logLine.Time = parsedTime

	if len(line) <= fabricmanagerLogTimeFormatN {
		return logLine
	}

	logLine.Content = strings.TrimSpace(line[fabricmanagerLogTimeFormatN:])
	return logLine
}
