package nvml

import (
	"database/sql"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/leptonai/gpud/pkg/sqlite"
)

type Op struct {
	db            *sql.DB
	gpmMetricsIDs map[nvml.GpmMetricId]struct{}
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	if op.db == nil {
		var err error
		op.db, err = sqlite.Open(":memory:")
		if err != nil {
			return err
		}
	}
	return nil
}

// Specifies the database instance to persist nvidia components data
// (e.g., xid/sxid events).
// If not specified, a new in-memory database is created.
func WithDB(db *sql.DB) OpOption {
	return func(op *Op) {
		op.db = db
	}
}

func WithGPMMetricsID(ids ...nvml.GpmMetricId) OpOption {
	return func(op *Op) {
		if op.gpmMetricsIDs == nil {
			op.gpmMetricsIDs = make(map[nvml.GpmMetricId]struct{})
		}
		for _, id := range ids {
			op.gpmMetricsIDs[id] = struct{}{}
		}
	}
}
