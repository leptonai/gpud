// Package fabricmanager tracks the NVIDIA fabric manager version and its activeness.
// And streams the fabric manager logs for any errors and events.
package fabricmanager

import (
	"context"
	"database/sql"
	"errors"
	"os/exec"
	"time"

	"github.com/leptonai/gpud/components"
	fabric_manager_id "github.com/leptonai/gpud/components/accelerator/nvidia/fabric-manager/id"
	"github.com/leptonai/gpud/components/systemd"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/log"
	pkg_systemd "github.com/leptonai/gpud/pkg/systemd"
)

func New(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB) (components.Component, error) {
	return newComponent(ctx, fabricManagerExists, defaultWatchCommands, dbRW, dbRO)
}

func newComponent(ctx context.Context, checkFMExists func() bool, watchCommands [][]string, dbRW *sql.DB, dbRO *sql.DB) (*component, error) {
	cctx, ccancel := context.WithCancel(ctx)

	var eventsStore events_db.Store
	var llp *logLineProcessor
	if checkFMExists() {
		var err error
		eventsStore, err = events_db.NewStore(
			dbRW,
			dbRO,
			events_db.CreateDefaultTableName(fabric_manager_id.Name),
			3*24*time.Hour,
		)
		if err != nil {
			ccancel()
			return nil, err
		}

		w, err := newWatcher(watchCommands)
		if err != nil {
			ccancel()
			return nil, err
		}
		llp = newLogLineProcessor(cctx, w, Match, eventsStore)
	}

	return &component{
		checkFMExists:    checkFMExists,
		rootCtx:          cctx,
		cancel:           ccancel,
		eventsStore:      eventsStore,
		logLineProcessor: llp,
	}, nil
}

var _ components.Component = (*component)(nil)

type component struct {
	checkFMExists    func() bool
	rootCtx          context.Context
	cancel           context.CancelFunc
	eventsStore      events_db.Store
	logLineProcessor *logLineProcessor
}

func (c *component) Name() string { return fabric_manager_id.Name }

func (c *component) Start() error { return nil }

func (c *component) States(ctx context.Context) ([]components.State, error) {
	if !c.checkFMExists() {
		return []components.State{
			{
				Name:    fabric_manager_id.Name,
				Health:  components.StateHealthy,
				Healthy: true,
				Reason:  "fabric manager not found",
			},
		}, nil
	}

	defaultConn := systemd.GetDefaultDbusConn()
	if defaultConn == nil {
		log.Logger.Errorw("systemd dbus connection not available")
		return nil, errors.New("systemd dbus connection not available")
	}

	active, err := checkFabricManagerActive(ctx, defaultConn)
	if err != nil {
		return nil, err
	}
	if !active {
		fmStatusOutput, err := pkg_systemd.GetLatestJournalctlOutput(ctx, "nvidia-fabricmanager")
		if err != nil {
			log.Logger.Errorw("failed to get latest fabric manager output", "error", err)
		} else {
			log.Logger.Warnw("fabric manager is not active", "output", fmStatusOutput)
		}
		return []components.State{
			{
				Name:    fabric_manager_id.Name,
				Health:  components.StateUnhealthy,
				Healthy: false,
				Reason:  "fabric manager found but not active",
			},
		}, nil
	}

	return []components.State{
		{
			Name:    fabric_manager_id.Name,
			Health:  components.StateHealthy,
			Healthy: true,
			Reason:  "fabric manager found and active",
		},
	}, nil
}

// Returns `github.com/leptonai/gpud/pkg/query.ErrNoData` if there is no event found.
func (c *component) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	if c.logLineProcessor != nil {
		return c.logLineProcessor.getEvents(ctx, since)
	}
	return nil, nil
}

func (c *component) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")
	c.cancel()

	if c.logLineProcessor != nil {
		c.logLineProcessor.close()
	}
	if c.eventsStore != nil {
		c.eventsStore.Close()
	}

	return nil
}

func fabricManagerExists() bool {
	p, err := exec.LookPath("nv-fabricmanager")
	if err != nil {
		return false
	}
	return p != ""
}

func checkFabricManagerActive(ctx context.Context, conn *pkg_systemd.DbusConn) (bool, error) {
	active, err := conn.IsActive(ctx, "nvidia-fabricmanager")
	if err != nil {
		return false, err
	}
	return active, nil
}
