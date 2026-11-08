package fabric

import (
	"context"
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
	_, _, err = r.Evaluate(context.Background(), now.Add(-time.Second), false, false)
	require.NoError(t, err)
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
