package kmsg

import (
	"context"

	"github.com/euank/go-kmsg-parser/v3/kmsgparser"

	events_db "github.com/leptonai/gpud/pkg/events-db"
)

type LogLineProcessor struct {
	ctx         context.Context
	kmsgWatcher kmsgparser.Parser
	matchFunc   MatchFunc
	eventsStore events_db.Store
}

type MatchFunc func(line string) (eventName string, message string)
