package fabric

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestRegistrationRecoveryAndTimeout(t *testing.T) {
	start := time.Now().UTC()
	var r Registration
	check := func(at time.Time, pending, failed bool, want apiv1.HealthStateType) {
		t.Helper()
		got, _, err := r.Evaluate(context.Background(), at, pending, failed)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	check(start, true, false, apiv1.HealthStateTypeDegraded)
	check(start.Add(77*time.Second), false, false, apiv1.HealthStateTypeHealthy)
	// Runtime re-registration starts a new bounded episode after full recovery.
	start = start.Add(time.Hour)
	check(start, true, false, apiv1.HealthStateTypeDegraded)
	check(start.Add(time.Minute), true, true, apiv1.HealthStateTypeUnhealthy)
	check(start.Add(2*time.Minute), false, true, apiv1.HealthStateTypeUnhealthy)
	check(start.Add(RegistrationTimeout-time.Nanosecond), true, false, apiv1.HealthStateTypeDegraded)
	check(start.Add(RegistrationTimeout), true, false, apiv1.HealthStateTypeUnhealthy)
	check(start.Add(72*time.Hour), true, false, apiv1.HealthStateTypeUnhealthy)
	check(start.Add(73*time.Hour), false, false, apiv1.HealthStateTypeHealthy)
}

func TestRegistrationPersistsAcrossRestartAndResetsAcrossBoot(t *testing.T) {
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	now := time.Now().UTC()
	check := func(boot string, at time.Time, pending bool, want apiv1.HealthStateType) {
		t.Helper()
		var r Registration
		require.NoError(t, r.Open(context.Background(), dbRW, "test", boot))
		got, _, err := r.Evaluate(context.Background(), at, pending, false)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	check("boot-a", now, true, apiv1.HealthStateTypeDegraded)
	check("boot-a", now.Add(RegistrationTimeout), true, apiv1.HealthStateTypeUnhealthy)
	check("boot-a", now.Add(RegistrationTimeout+time.Second), false, apiv1.HealthStateTypeHealthy)
	check("boot-a", now.Add(RegistrationTimeout+2*time.Second), true, apiv1.HealthStateTypeDegraded)
	check("boot-b", now.Add(time.Hour), true, apiv1.HealthStateTypeDegraded)
}

func TestRegistrationPersistenceFailureCannotGrantGrace(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		dbRW, _, cleanup := sqlite.OpenTestDB(t)
		defer cleanup()
		var r Registration
		require.NoError(t, r.Open(context.Background(), dbRW, "test", "boot"))
		r.loaded = loaded
		require.NoError(t, dbRW.Close())
		got, _, err := r.Evaluate(context.Background(), time.Now(), true, false)
		require.Error(t, err)
		require.Equal(t, apiv1.HealthStateTypeUnhealthy, got)
	}
}

func TestRegistrationStaleCheckCannotResetDeadline(t *testing.T) {
	var r Registration
	now := time.Now()
	_, _, err := r.Evaluate(context.Background(), now, true, false)
	require.NoError(t, err)
	stale, _, err := r.Evaluate(context.Background(), now.Add(-time.Second), false, false)
	require.NoError(t, err)
	require.Equal(t, apiv1.HealthStateTypeDegraded, stale)
	got, _, err := r.Evaluate(context.Background(), now.Add(RegistrationTimeout), true, false)
	require.NoError(t, err)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, got)
}

func TestRegistrationSubsecondPersistence(t *testing.T) {
	dbRW, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	now := time.Now().Truncate(time.Second)
	for i, pending := range []bool{true, false, true, false} {
		var r Registration
		require.NoError(t, r.Open(context.Background(), dbRW, "test", "boot"))
		got, _, err := r.Evaluate(context.Background(), now.Add(time.Duration(i)*time.Millisecond), pending, false)
		require.NoError(t, err)
		if pending {
			require.Equal(t, apiv1.HealthStateTypeDegraded, got)
		} else {
			require.Equal(t, apiv1.HealthStateTypeHealthy, got)
		}
	}
	var r Registration
	require.NoError(t, r.Open(context.Background(), dbRW, "test", "boot"))
	got, _, err := r.Evaluate(context.Background(), now.Add(time.Hour), true, false)
	require.NoError(t, err)
	require.Equal(t, apiv1.HealthStateTypeDegraded, got)
}

func TestRegistrationConcurrentChecks(t *testing.T) {
	var r Registration
	now := time.Now()
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, _, err := r.Evaluate(context.Background(), now, true, false)
			require.NoError(t, err)
		})
	}
	wg.Wait()
	got, _, err := r.Evaluate(context.Background(), now.Add(RegistrationTimeout), true, false)
	require.NoError(t, err)
	require.Equal(t, apiv1.HealthStateTypeUnhealthy, got)
}

func TestRegistrationRestoresMonotonicDeadline(t *testing.T) {
	for _, offset := range []time.Duration{-time.Hour, time.Hour} {
		t.Run(offset.String(), func(t *testing.T) {
			db, _, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()
			now := time.Now()
			var old Registration
			require.NoError(t, old.Open(context.Background(), db, "test", "boot"))
			_, _, err := old.Evaluate(context.Background(), now.Add(offset), true, false)
			require.NoError(t, err)
			var restored Registration
			require.NoError(t, restored.Open(context.Background(), db, "test", "boot"))
			health, _, err := restored.Evaluate(context.Background(), now, true, false)
			require.NoError(t, err)
			require.NotEqual(t, restored.since, restored.since.Round(0), "restored deadline must retain a monotonic reading")
			if offset < 0 {
				require.Equal(t, apiv1.HealthStateTypeUnhealthy, health)
			} else {
				require.Equal(t, apiv1.HealthStateTypeDegraded, health)
				require.True(t, restored.since.Equal(now))
			}
			health, _, err = restored.Evaluate(context.Background(), now.Add(time.Second), false, false)
			require.NoError(t, err)
			require.Equal(t, apiv1.HealthStateTypeHealthy, health)
		})
	}
}

func TestRegistrationClampedCheckpointSurvivesRepeatedRestarts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "registration.db")
	now := time.Now()
	db, err := sqlite.Open(path)
	require.NoError(t, err)
	var original Registration
	require.NoError(t, original.Open(ctx, db, "test", "boot"))
	_, _, err = original.Evaluate(ctx, now.Add(time.Hour), true, false)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Every observation opens a new connection and Registration, as a same-boot
	// daemon restart does. Restart both before and after the corrected deadline.
	for _, elapsed := range []time.Duration{0, 4 * time.Minute, 5 * time.Minute, 6 * time.Minute, 8 * time.Minute, 16 * time.Minute} {
		db, err := sqlite.Open(path)
		require.NoError(t, err)
		var restored Registration
		require.NoError(t, restored.Open(ctx, db, "test", "boot"))
		health, _, err := restored.Evaluate(ctx, now.Add(elapsed), true, false)
		require.NoError(t, err)
		want := apiv1.HealthStateTypeDegraded
		if elapsed >= RegistrationTimeout {
			want = apiv1.HealthStateTypeUnhealthy
		}
		require.Equal(t, want, health, "elapsed %s", elapsed)
		var stored string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT since FROM nv_fabric_registration WHERE component = ?", "test").Scan(&stored))
		require.Equal(t, now.Format(time.RFC3339Nano), stored, "normalized checkpoint must survive restart")
		require.NotEqual(t, restored.since, restored.since.Round(0), "retain the monotonic anchor")
		require.NoError(t, db.Close())
	}
}

func TestRegistrationClampSaveFailureCannotGrantGrace(t *testing.T) {
	ctx := context.Background()
	db, _, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	now := time.Now()
	var original Registration
	require.NoError(t, original.Open(ctx, db, "test", "boot"))
	future := now.Add(time.Hour)
	_, _, err := original.Evaluate(ctx, future, true, false)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TRIGGER reject_checkpoint BEFORE INSERT ON nv_fabric_registration BEGIN SELECT RAISE(ABORT, 'checkpoint write blocked'); END`)
	require.NoError(t, err)
	var restored Registration
	require.NoError(t, restored.Open(ctx, db, "test", "boot"))
	for range 2 {
		health, reason, err := restored.Evaluate(ctx, now, true, false)
		require.ErrorContains(t, err, "checkpoint write blocked")
		require.Equal(t, apiv1.HealthStateTypeUnhealthy, health)
		require.Contains(t, reason, "cannot persist")
		require.False(t, restored.loaded, "failed normalization must be retried")
	}
	var stored string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT since FROM nv_fabric_registration WHERE component = ?", "test").Scan(&stored))
	require.Equal(t, future.Format(time.RFC3339Nano), stored)
	_, err = db.ExecContext(ctx, "DROP TRIGGER reject_checkpoint")
	require.NoError(t, err)
	health, _, err := restored.Evaluate(ctx, now, true, false)
	require.NoError(t, err)
	require.Equal(t, apiv1.HealthStateTypeDegraded, health)
	require.True(t, restored.loaded)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT since FROM nv_fabric_registration WHERE component = ?", "test").Scan(&stored))
	require.Equal(t, now.Format(time.RFC3339Nano), stored)
}
