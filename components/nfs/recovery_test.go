package nfs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

func recoveryComponent(t *testing.T) (*component, *nfsMockEventBucket) {
	t.Helper()
	bucket := &nfsMockEventBucket{events: eventstore.Events{{Name: eventNFSServerNotResponding, Message: messageNFSServerNotResponding + ": 192.0.2.1", Time: time.Now().Add(-20 * time.Minute)}}}
	c := newKmsgComponent(bucket, &nfsMockRebootEventStore{})
	c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
		return pkgnfschecker.Configs{{VolumePath: "/nfs", DirName: ".check", FileContents: "check"}}
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, bucket
}

// Reproduce the missing kernel OK: unchanged historical evidence must not
// prevent a successful, fresh I/O probe from establishing current health.
func TestRecoveryWithoutKernelOK(t *testing.T) {
	c, bucket := recoveryComponent(t)
	calls := 0
	c.recoverNFS = func(ctx context.Context, req pkgnfschecker.RecoveryRequest) error {
		_, deadline := ctx.Deadline()
		require.True(t, deadline)
		require.Equal(t, []string{"192.0.2.1"}, req.Servers)
		require.Len(t, req.Configs, 1)
		calls++
		if calls == 1 {
			return errors.New("server unavailable")
		}
		return nil
	}
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
	require.Equal(t, 3, calls, "never reuse a previous success")
	require.Len(t, bucket.events, 1, "keep warning history")
	require.Zero(t, bucket.lastPurgeBefore)
	// A subsequent outage must be observable even without another kernel message.
	c.recoverNFS = func(context.Context, pkgnfschecker.RecoveryRequest) error { return context.DeadlineExceeded }
	require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
}

func TestRecoveryRetainsOtherFailures(t *testing.T) {
	for _, name := range []string{eventNFSLockReclaimFailed, eventNFSWritebackHang} {
		t.Run(name, func(t *testing.T) {
			c, bucket := recoveryComponent(t)
			bucket.events = append(bucket.events, eventstore.Event{Name: name, Time: time.Now()}, eventstore.Event{Name: name, Time: time.Now()})
			c.recoverNFS = func(context.Context, pkgnfschecker.RecoveryRequest) error {
				t.Fatal("must not override unrelated failures")
				return nil
			}
			require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
		})
	}
}

func TestRecoveryRejectsNewEvents(t *testing.T) {
	for _, name := range []string{eventNFSServerNotResponding, eventNFSLockReclaimFailed, eventNFSWritebackHang} {
		t.Run(name, func(t *testing.T) {
			c, bucket := recoveryComponent(t)
			c.recoverNFS = func(ctx context.Context, _ pkgnfschecker.RecoveryRequest) error {
				for range 2 {
					require.NoError(t, bucket.Insert(ctx, eventstore.Event{Name: name, Message: messageNFSServerNotResponding + ": 192.0.2.2", Time: time.Now()}))
				}
				return nil
			}
			result := c.Check()
			require.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
			require.Contains(t, result.Summary(), "new NFS issue")
		})
	}
}

func TestRecoveryUnavailableEvidence(t *testing.T) {
	for _, kind := range []string{"no configs", "unknown server", "event read failed", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			c, bucket := recoveryComponent(t)
			c.recoverNFS = func(context.Context, pkgnfschecker.RecoveryRequest) error {
				if kind == "event read failed" {
					bucket.getErr = errors.New("database unavailable")
				}
				if kind == "canceled" {
					c.cancel()
				}
				return nil
			}
			if kind == "no configs" {
				c.getGroupConfigsFunc = func() pkgnfschecker.Configs { return nil }
			}
			if kind == "unknown server" {
				bucket.events[0].Message = messageNFSServerNotResponding
			}
			require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
		})
	}
}

func TestRecoveryIgnoresAlreadyExcludedPreRebootEvents(t *testing.T) {
	c, bucket := recoveryComponent(t)
	bucket.events = append(bucket.events, eventstore.Event{Name: eventNFSLockReclaimFailed, Time: time.Now().Add(-2 * time.Hour)})
	c.rebootEventStore = &nfsMockRebootEventStore{events: eventstore.Events{{Time: time.Now().Add(-time.Hour)}}}
	c.recoverNFS = func(context.Context, pkgnfschecker.RecoveryRequest) error { return nil }
	require.Equal(t, apiv1.HealthStateTypeHealthy, c.Check().HealthStateType())
}

func TestRecoveryRejectsConfigChange(t *testing.T) {
	c, _ := recoveryComponent(t)
	c.recoverNFS = func(context.Context, pkgnfschecker.RecoveryRequest) error {
		c.getGroupConfigsFunc = func() pkgnfschecker.Configs { return nil }
		return nil
	}
	result := c.Check()
	require.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
	require.Contains(t, result.Summary(), "configuration changed")
}

func TestOutstandingRecoveryBlocksOrdinaryProbes(t *testing.T) {
	patch := mockey.Mock((*pkgnfschecker.RecoveryRunner).IsRunning).Return(true).Build()
	defer patch.UnPatch()
	for _, state := range []string{"kernel OK", "manual reset"} {
		t.Run(state, func(t *testing.T) {
			c, bucket := recoveryComponent(t)
			if state == "kernel OK" {
				bucket.events = append(bucket.events, eventstore.Event{Name: eventNFSServerOK, Message: messageNFSServerOK + ": 192.0.2.1", Time: time.Now()})
			} else {
				bucket.events = nil
			}
			c.getGroupConfigsFunc = func() pkgnfschecker.Configs {
				t.Fatal("ordinary probes must not bypass blocked recovery I/O")
				return nil
			}
			result := c.Check()
			require.Equal(t, apiv1.HealthStateTypeDegraded, result.HealthStateType())
			require.Contains(t, result.Summary(), "still running")
		})
	}
}
