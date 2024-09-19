// Package server implements a process run server.
package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/process/state"
	"github.com/leptonai/gpud/pkg/process/state/schema"
	state_sqlite "github.com/leptonai/gpud/pkg/process/state/sqlite"
)

type Config struct {
	SQLite    *sql.DB
	TableName string
}

type Server struct {
	state state.Interface
	cfg   Config
}

func New(cfg Config) (*Server, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	state, err := state_sqlite.New(ctx, cfg.SQLite, cfg.TableName)
	cancel()
	if err != nil {
		return nil, err
	}
	return &Server{
		state: state,
		cfg:   cfg,
	}, nil
}

// TODO rate limit

// Starts the script and returns the id.
func (s *Server) Start(ctx context.Context, script string, scriptName string) (string, error) {
	// use sha256
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(script)))

	rerr := s.state.RecordStart(ctx, script, scriptName)
	if rerr != nil {
		return id, rerr
	}

	// TODO: run the script in the background

	return id, nil
}

func (s *Server) Check(ctx context.Context, id string) (*schema.Row, error) {
	return s.state.Get(ctx, id)
}
