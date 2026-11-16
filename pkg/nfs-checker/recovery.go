package nfschecker

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// RecoveryRequest contains only explicitly configured writable probe locations.
type RecoveryRequest struct {
	Configs  Configs
	HostRoot string
	Servers  []string
}

// RecoveryRunner holds one worker slot through I/O and cleanup, even after
// cancellation: contexts cannot interrupt NFS syscalls. Late results are discarded.
type RecoveryRunner struct {
	mu      sync.Mutex
	running bool
	probe   func(context.Context, RecoveryRequest) error
}

// IsRunning also prevents ordinary probes from overlapping a timed-out worker.
func (r *RecoveryRunner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *RecoveryRunner) Run(ctx context.Context, req RecoveryRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("previous NFS recovery probe is still running")
	}
	r.running = true
	r.mu.Unlock()
	// The caller can return before I/O does. Own the request slices so later
	// configuration changes cannot change what the outstanding worker is probing.
	req.Configs = slices.Clone(req.Configs)
	req.Servers = slices.Clone(req.Servers)
	probe := r.probe
	if probe == nil {
		probe = func(ctx context.Context, req RecoveryRequest) error {
			return runRecoveryProbe(ctx, req, readRecoveryMounts)
		}
	}
	done := make(chan error, 1)
	go func() {
		err := probe(ctx, req)
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
		done <- err
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		// If cancellation and completion race, expired evidence must not establish
		// Healthy. A later check always starts a fresh probe with its own channel.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
}
