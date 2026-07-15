package nvlink

import (
	"fmt"
	"regexp"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	eventNameDriverWedge = "nvlink_driver_wedge"
	driverWedgeReason    = "NVIDIA driver is stuck retrying NVLink link discovery (postRxDetLinkMask failure)"
)

var driverWedgePattern = regexp.MustCompile(`NVRM: knvlinkDiscoverPostRxDetLinks_[A-Za-z0-9_]+: Getting peer[0-9]+(?:'s)? postRxDetLinkMask failed!`)

func matchDriverWedge(line string) bool {
	return driverWedgePattern.MatchString(line)
}

func (c *component) startDriverWedgeDetection() error {
	if c.eventBucket != nil {
		if err := c.restoreDriverWedge(); err != nil {
			// The watcher reads the current boot's kmsg buffer, so a transient
			// restore failure does not disable live detection.
			log.Logger.Warnw("failed to restore nvlink driver wedge event", "error", err)
		}
	}
	if c.kmsgWatcher == nil {
		return nil
	}

	messages, err := c.kmsgWatcher.Watch()
	if err != nil {
		return fmt.Errorf("failed to watch kmsg for nvlink driver wedge: %w", err)
	}
	go c.watchDriverWedge(messages)
	return nil
}

func (c *component) watchDriverWedge(messages <-chan kmsg.Message) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case message, ok := <-messages:
			if !ok {
				return
			}
			if matchDriverWedge(message.Message) {
				c.recordDriverWedge(message)
			}
		}
	}
}

func (c *component) recordDriverWedge(message kmsg.Message) {
	detectedAt := message.Timestamp.Time.UTC()
	if detectedAt.IsZero() {
		detectedAt = c.getTimeNowFunc()
	}

	c.lastMu.Lock()
	if c.driverWedgeDetectedAt.IsZero() {
		c.driverWedgeDetectedAt = detectedAt
		c.driverWedgeMessage = driverWedgeReason
	}
	if c.eventBucket == nil || c.driverWedgePersisted {
		c.lastMu.Unlock()
		return
	}
	// Reserve the insert so repeated four-second kernel messages cannot race
	// each other into duplicate persisted events.
	c.driverWedgePersisted = true
	c.lastMu.Unlock()

	event := eventstore.Event{
		Component: Name,
		Time:      detectedAt,
		Name:      eventNameDriverWedge,
		Type:      string(apiv1.EventTypeFatal),
		Message:   driverWedgeReason,
	}
	if err := c.eventBucket.Insert(c.ctx, event); err != nil {
		c.lastMu.Lock()
		c.driverWedgePersisted = false
		c.lastMu.Unlock()
		log.Logger.Errorw("failed to persist nvlink driver wedge event", "error", err)
		return
	}
	log.Logger.Errorw("detected nvlink driver wedge", "event", eventNameDriverWedge, "detectedAt", detectedAt)
}

func (c *component) restoreDriverWedge() error {
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
		if event.Name != eventNameDriverWedge {
			continue
		}
		message := event.Message
		if message == "" {
			message = driverWedgeReason
		}
		c.lastMu.Lock()
		c.driverWedgeDetectedAt = event.Time.UTC()
		c.driverWedgeMessage = message
		c.driverWedgePersisted = true
		c.lastMu.Unlock()
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

func (c *component) healthOverride() *apiv1.HealthState {
	now := c.getTimeNowFunc()

	c.lastMu.RLock()
	wedgeDetectedAt := c.driverWedgeDetectedAt
	wedgeMessage := c.driverWedgeMessage
	monitoringStartedAt := c.monitoringStartedAt
	lastCheckCompletedAt := c.lastCheckCompletedAt
	var oldestCheck time.Time
	for _, startedAt := range c.checksInFlight {
		if oldestCheck.IsZero() || startedAt.Before(oldestCheck) {
			oldestCheck = startedAt
		}
	}
	c.lastMu.RUnlock()

	if !wedgeDetectedAt.IsZero() {
		return unhealthyRebootState(wedgeDetectedAt, wedgeMessage)
	}
	if !oldestCheck.IsZero() && now.Sub(oldestCheck) >= checkStaleAfter {
		reason := fmt.Sprintf("NVIDIA NVLink health check has not completed for %s; NVIDIA driver may be unresponsive", now.Sub(oldestCheck).Round(time.Second))
		return unhealthyRebootState(oldestCheck, reason)
	}
	if oldestCheck.IsZero() && !monitoringStartedAt.IsZero() {
		lastRefresh := lastCheckCompletedAt
		if lastRefresh.IsZero() {
			lastRefresh = monitoringStartedAt
		}
		if now.Sub(lastRefresh) >= checkStaleAfter {
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
