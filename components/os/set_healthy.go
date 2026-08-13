//nolint:revive // package name follows the directory import path used across the codebase.
package os

import (
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
)

var _ components.HealthSettable = &component{}

// SetHealthy resets the D-state (blocked) process tracking state (LEP-6029).
//
// Unlike the xid/sxid components, this must NOT purge the event bucket: the
// "os" bucket is shared with reboot events (pkghost.EventBucketName) that
// reboot tracking and the xid component's escalation counting depend on. The
// os health state is recomputed live on every Check from the current process
// table, so purging history is unnecessary; instead, any active
// blocked-process episode is closed (recording the recovery event that the
// reboot-escalation window anchors on) and persistence tracking restarts from
// scratch, giving the operator's fix the full persistence-threshold window to
// take effect before the component can flag again.
func (c *component) SetHealthy() error {
	log.Logger.Infow("set healthy event received for os")

	now := time.Now().UTC()
	if c.getTimeNowFunc != nil {
		now = c.getTimeNowFunc()
	}

	// inserts EventNameBlockedProcessesRecovered when an episode is active,
	// which also resets the reboot-escalation window (reboots are counted only
	// since the most recent recovery)
	c.clearBlockedProcessEpisode(now, "persistent blocked processes cleared (set-healthy)")

	// forget all tracked D-state processes: a process still blocked must
	// re-earn the persistence threshold before being flagged again
	if c.blockedTracker != nil {
		c.blockedTracker.reset()
	}

	// no synchronous re-check: Check enumerates all processes and may take
	// seconds, which would stall the session reader loop
	// (pkg/session.processSetHealthy); the next periodic check (one-minute
	// cadence) reflects the reset state
	return nil
}
