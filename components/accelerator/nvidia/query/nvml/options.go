package nvml

import (
	"database/sql"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/leptonai/gpud/pkg/sqlite"
)

type Op struct {
	dbRW          *sql.DB
	dbRO          *sql.DB
	gpmMetricsIDs map[nvml.GpmMetricId]struct{}
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) error {
	for _, opt := range opts {
		opt(op)
	}
	if op.dbRW == nil {
		var err error
		op.dbRW, err = sqlite.Open(":memory:")
		if err != nil {
			return err
		}
	}
	if op.dbRO == nil {
		var err error
		op.dbRO, err = sqlite.Open(":memory:", sqlite.WithReadOnly(true))
		if err != nil {
			return err
		}
	}
	return nil
}

// Specifies the database instance to persist nvidia components data
// (e.g., xid/sxid events). Must be a writable database instance.
// If not specified, a new in-memory database is created.
func WithDBRW(db *sql.DB) OpOption {
	return func(op *Op) {
		op.dbRW = db
	}
}

// Specifies the read-only database instance.
// If not specified, a new in-memory database is created.
func WithDBRO(db *sql.DB) OpOption {
	return func(op *Op) {
		op.dbRO = db
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
