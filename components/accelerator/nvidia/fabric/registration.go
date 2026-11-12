package fabric

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// RegistrationTimeout bounds a continuously incomplete fabric registration.
// This is an observation window, not a delay applied to every GPU or every boot.
const RegistrationTimeout = 5 * time.Minute

// Registration remembers an incomplete registration across checks and daemon
// restarts. Only a complete, healthy observation or a new boot resets the window.
// The zero value supports one-shot checks without persistent storage.
type Registration struct {
	mu                  sync.Mutex
	db                  *sql.DB
	component, bootID   string
	loaded              bool
	since, lastObserved time.Time
	lastHealth          apiv1.HealthStateType
	lastReason          string
}

// Open uses the daemon's existing database. One checkpoint per component is
// retained independently of diagnostic event retention. The caller owns db.
func (r *Registration) Open(ctx context.Context, db *sql.DB, component, bootID string) error {
	r.db, r.component, r.bootID = db, component, bootID
	if db == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS nv_fabric_registration (
 component TEXT PRIMARY KEY, boot_id TEXT NOT NULL, since TEXT NOT NULL)`)
	return err
}

// Evaluate requires timestamps retaining their monotonic reading (time.Now, not UTC).
// In-process wall-clock corrections therefore do not alter the observation window.
// Evaluate never grants grace for a query error or an explicit fault. Degraded
// describes observed unavailability without retaining an older Unhealthy state
// in consumers that interpret Initializing as "no new observation".
func (r *Registration) Evaluate(ctx context.Context, now time.Time, pending, failed bool) (health apiv1.HealthStateType, reason string, retErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer func() { r.lastHealth, r.lastReason = health, reason }()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if !r.loaded {
		if r.db != nil {
			var bootID, since string
			err := r.db.QueryRowContext(ctx, `SELECT boot_id, since FROM nv_fabric_registration WHERE component = ?`, r.component).Scan(&bootID, &since)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return apiv1.HealthStateTypeUnhealthy, "cannot read fabric registration deadline", err
			}
			if err == nil && bootID == r.bootID && since != "" {
				r.since, err = time.Parse(time.RFC3339Nano, since)
				if err != nil {
					return apiv1.HealthStateTypeUnhealthy, "invalid fabric registration deadline", err
				}
			}
		}
		// Re-anchor the persisted wall timestamp to this process monotonic clock.
		// A backward correction across a restart cannot start the window in the future.
		if !r.since.IsZero() {
			// Persist a backward-clock correction before granting grace, so
			// another same-boot restart cannot move the deadline again.
			if now.Before(r.since) {
				if err := r.save(ctx, now); err != nil {
					return apiv1.HealthStateTypeUnhealthy, "cannot persist normalized fabric registration deadline", err
				}
			}
			elapsed := max(now.Sub(r.since), 0)
			r.since = now.Add(-elapsed)
		}
		r.loaded = true
	}
	// Concurrent checks may finish out of order. An older healthy sample must
	// not clear the deadline established by a newer incomplete observation.
	if now.Before(r.lastObserved) {
		return r.lastHealth, r.lastReason, nil
	}
	r.lastObserved = now
	if pending && r.since.IsZero() {
		if err := r.save(ctx, now); err != nil {
			return apiv1.HealthStateTypeUnhealthy, "cannot persist fabric registration deadline", err
		}
		r.since = now
	}
	if failed {
		return apiv1.HealthStateTypeUnhealthy, "fabric registration or health check failed", nil
	}
	if !pending {
		if !r.since.IsZero() {
			if err := r.save(ctx, time.Time{}); err != nil {
				return apiv1.HealthStateTypeUnhealthy, "cannot persist fabric registration completion", err
			}
			r.since = time.Time{}
		}
		return apiv1.HealthStateTypeHealthy, "fabric registration completed", nil
	}
	if !now.Before(r.since.Add(RegistrationTimeout)) {
		return apiv1.HealthStateTypeUnhealthy, fmt.Sprintf("fabric registration did not complete within %s (first observed %s)", RegistrationTimeout, r.since.Format(time.RFC3339)), nil
	}
	return apiv1.HealthStateTypeDegraded, fmt.Sprintf("fabric registration in progress (deadline %s)", r.since.Add(RegistrationTimeout).Format(time.RFC3339)), nil
}

func (r *Registration) save(ctx context.Context, since time.Time) error {
	if r.db == nil {
		return nil
	}
	value := ""
	if !since.IsZero() {
		value = since.Format(time.RFC3339Nano)
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO nv_fabric_registration (component, boot_id, since)
 VALUES (?, ?, ?) ON CONFLICT(component) DO UPDATE SET boot_id = excluded.boot_id, since = excluded.since`, r.component, r.bootID, value)
	return err
}
