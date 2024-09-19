// Package server implements a process run server.
package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/process/state"
	"github.com/leptonai/gpud/pkg/process/state/schema"
	state_sqlite "github.com/leptonai/gpud/pkg/process/state/sqlite"
	"tailscale.com/tstime/rate"
)

var (
	ErrQPSLimitExceeded     = errors.New("qps limit exceeded")
	ErrMinimumRetryInterval = errors.New("minimum retry interval not yet met -- try again later")
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

type Server struct {
	state       state.Interface
	rateLimiter *rate.Limiter
	cfg         Config
}

func New(cfg Config) (*Server, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	state, err := state_sqlite.New(ctx, cfg.SQLite, cfg.TableName)
	cancel()
	if err != nil {
		return nil, err
	}

	srv := &Server{
		state: state,
		cfg:   cfg,
	}
	if cfg.QPS > 0 {
		srv.rateLimiter = rate.NewLimiter(rate.Limit(cfg.QPS), cfg.QPS)
	}

	return srv, nil
}

// Starts the script and returns the id.
func (s *Server) Start(ctx context.Context, script string) (string, error) {
	if s.rateLimiter != nil && !s.rateLimiter.Allow() {
		return "", ErrQPSLimitExceeded
	}

	id := CreateID(script)
	prev, err := s.state.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if prev != nil {
		now := time.Now().UTC().Unix()
		elapsed := now - prev.LastStartedUnixSeconds
		if elapsed < s.cfg.MinimumRetryIntervalSeconds {
			return "", ErrMinimumRetryInterval
		}
	}

	if rerr := s.state.RecordStart(ctx, id); rerr != nil {
		return id, rerr
	}

	// TODO: run the script in the background

	return id, nil
}

func (s *Server) Check(ctx context.Context, id string) (*schema.Status, error) {
	if s.rateLimiter != nil && !s.rateLimiter.Allow() {
		return nil, ErrQPSLimitExceeded
	}

	return s.state.Get(ctx, id)
}

// Derives the id from the script contents.
func CreateID(scriptContents string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(scriptContents)))
}
