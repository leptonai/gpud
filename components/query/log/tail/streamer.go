package tail

import (
	query_log_filter "github.com/leptonai/gpud/components/query/log/filter"

	"github.com/nxadm/tail"
)

// Streamer defines the log tailer.
type Streamer interface {
	// Returns the file that the streamer watches on.
	File() string
	// Returns the command arguments that the streamer watches on.
	Commands() [][]string

	// Returns the line channel that the streaming lines are sent to.
	Line() <-chan Line
}

type Line struct {
	*tail.Line
	MatchedFilter *query_log_filter.Filter
}
