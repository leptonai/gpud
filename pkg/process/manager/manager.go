// Package manager implements a process run manager.
package manager

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/process/manager/state"
	"github.com/leptonai/gpud/pkg/process/manager/state/schema"
	state_sqlite "github.com/leptonai/gpud/pkg/process/manager/state/sqlite"

	"tailscale.com/tstime/rate"
)

type Config struct {
	SQLite    *sql.DB
	TableName string

	// QPS is the maximum number of requests per second.
	QPS int

	// MinimumRetryIntervalSeconds is the minimum number of seconds between retries.
	// If the same script is requested to start within this interval, the request will be rejected.
	MinimumRetryIntervalSeconds int64
}

type Manager interface {
	// Starts the script and returns the id and the created process.
	StartBashScript(ctx context.Context, scriptContents string, opts ...process.OpOption) (string, process.Process, error)

	// Get returns the status of the process with the given id.
	// Returns status nil, error ErrNotFound if the script id does not exist.
	Get(ctx context.Context, id string) (*schema.Status, error)
}

type manager struct {
	state       state.Interface
	rateLimiter *rate.Limiter
	cfg         Config
}

func New(cfg Config) (Manager, error) {
	if cfg.SQLite == nil {
		return nil, errors.New("sqlite is not set")
	}
	if cfg.TableName == "" {
		return nil, errors.New("table name is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	stateInterface, err := state_sqlite.New(ctx, cfg.SQLite, cfg.TableName)
	cancel()
	if err != nil {
		return nil, err
	}

	mngr := &manager{
		state: stateInterface,
		cfg:   cfg,
	}
	if cfg.QPS > 0 {
		mngr.rateLimiter = rate.NewLimiter(rate.Limit(cfg.QPS), cfg.QPS)
	}

	return mngr, nil
}

var (
	ErrQPSLimitExceeded     = errors.New("qps limit exceeded")
	ErrMinimumRetryInterval = errors.New("minimum retry interval not yet met -- try again later")
)

// Starts the script and returns the id and the created process.
func (s *manager) StartBashScript(ctx context.Context, scriptContents string, opts ...process.OpOption) (string, process.Process, error) {
	if s.rateLimiter != nil && !s.rateLimiter.Allow() {
		return "", nil, ErrQPSLimitExceeded
	}

	id := CreateID(scriptContents)
	prev, err := s.state.Get(ctx, id)
	if err != nil && err != state.ErrNotFound {
		return "", nil, err
	}
	if prev != nil {
		now := time.Now().UTC().Unix()
		elapsed := now - prev.LastStartedUnixSeconds
		if elapsed < s.cfg.MinimumRetryIntervalSeconds {
			return "", nil, ErrMinimumRetryInterval
		}
		// same command has been run before, but enough interval has elapsed
		// so we can run it again
	}

	if rerr := s.state.RecordStart(ctx, id); rerr != nil {
		return id, nil, rerr
	}

	// append "WithBashScriptContentsToRun" at the end to overwrie any conflicting options before
	opts = append(opts, process.WithBashScriptContentsToRun(scriptContents))
	proc, err := process.New(opts...)
	if err != nil {
		return "", nil, err
	}
	if err := proc.Start(ctx); err != nil {
		return "", proc, err
	}
	return id, proc, nil
}

// Get returns the status of the process with the given id.
// Returns status nil, error ErrNotFound if the script id does not exist.
func (s *manager) Get(ctx context.Context, id string) (*schema.Status, error) {
	if s.rateLimiter != nil && !s.rateLimiter.Allow() {
		return nil, ErrQPSLimitExceeded
	}
	return s.state.Get(ctx, id)
}

// Derives the id from the script contents.
func CreateID(scriptContents string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(scriptContents)))
}
