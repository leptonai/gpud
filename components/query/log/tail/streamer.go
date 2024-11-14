package tail

import (
	"sync"

	query_log_common "github.com/leptonai/gpud/components/query/log/common"

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
	MatchedFilter *query_log_common.Filter
}

type streamDeduper struct {
	seen map[string]struct{}
	mu   sync.Mutex
}

var seenPool = sync.Pool{
	New: func() interface{} {
		return &streamDeduper{
			seen: make(map[string]struct{}, 1000),
		}
	},
}
