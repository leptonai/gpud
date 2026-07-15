package nvlink

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

func (c *component) updateCurrentState() error {
	if c.eventBucket == nil {
		return nil
	}

	bootTime := c.getBootTimeFunc()
	now := c.getTimeNowFunc()
	if bootTime.Unix() <= 0 || bootTime.After(now) {
		log.Logger.Warnw("skipping nvlink driver wedge restore due to invalid boot time", "bootTime", bootTime, "now", now)
		return nil
	}

	events, err := c.eventBucket.Get(c.ctx, bootTime.Add(-time.Second))
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Name != EventNameDriverWedge {
			continue
		}
		c.mu.Lock()
		c.currState = *unhealthyRebootState(event.Time.UTC(), driverWedgeMessage)
		c.mu.Unlock()
		return nil
	}
	return nil
}

func (c *component) beginCheck(cr *checkResult) {
	c.lastMu.Lock()
	if c.checksInFlight == nil {
		c.checksInFlight = make(map[*checkResult]time.Time)
	}
	c.checksInFlight[cr] = cr.ts
	c.lastMu.Unlock()
}

func (c *component) finishCheck(cr *checkResult) {
	completedAt := c.getTimeNowFunc()
	c.lastMu.Lock()
	c.lastCheckResult = cr
	c.lastCheckCompletedAt = completedAt
	delete(c.checksInFlight, cr)
	c.lastMu.Unlock()
}

func (c *component) markMonitoringStarted() {
	startedAt := c.getTimeNowFunc()
	c.lastMu.Lock()
	if c.monitoringStartedAt.IsZero() {
		c.monitoringStartedAt = startedAt
	}
	c.lastMu.Unlock()
}

func (c *component) watchdogHealthState() *apiv1.HealthState {
	now := c.getTimeNowFunc()

	c.lastMu.RLock()
	monitoringStartedAt := c.monitoringStartedAt
	lastCheckCompletedAt := c.lastCheckCompletedAt
	var oldestCheck time.Time
	for _, startedAt := range c.checksInFlight {
		if oldestCheck.IsZero() || startedAt.Before(oldestCheck) {
			oldestCheck = startedAt
		}
	}
	c.lastMu.RUnlock()

	if !oldestCheck.IsZero() && now.Sub(oldestCheck) >= defaultCheckStaleAfter {
		reason := fmt.Sprintf("NVIDIA NVLink health check has not completed for %s; NVIDIA driver may be unresponsive", now.Sub(oldestCheck).Round(time.Second))
		return unhealthyRebootState(oldestCheck, reason)
	}
	if oldestCheck.IsZero() && !monitoringStartedAt.IsZero() {
		lastRefresh := lastCheckCompletedAt
		if lastRefresh.IsZero() {
			lastRefresh = monitoringStartedAt
		}
		if now.Sub(lastRefresh) >= defaultCheckStaleAfter {
			reason := fmt.Sprintf("NVIDIA NVLink health state has not refreshed for %s; NVIDIA driver may be unresponsive", now.Sub(lastRefresh).Round(time.Second))
			return unhealthyRebootState(lastRefresh, reason)
		}
	}
	return nil
}

func unhealthyRebootState(timestamp time.Time, reason string) *apiv1.HealthState {
	return &apiv1.HealthState{
		Time:      metav1.NewTime(timestamp),
		Component: Name,
		Name:      Name,
		Health:    apiv1.HealthStateTypeUnhealthy,
		Reason:    reason,
		SuggestedActions: &apiv1.SuggestedActions{
			Description: reason,
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		},
	}
}
