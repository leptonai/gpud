package events

import (
	"context"
	"fmt"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

var _ RebootsStore = &rebootsStore2{}

type rebootsStore2 struct {
	getTimeNowFunc    func() time.Time
	getLastRebootTime func(context.Context) (time.Time, error)
	bucket            eventstore.Bucket
}

func (s *rebootsStore2) Record(ctx context.Context) error {
	currentBootTime, err := s.getLastRebootTime(ctx)
	if err != nil {
		return err
	}

	// if now - event time > retention (too old), then skip
	now := s.getTimeNowFunc()
	if now.Sub(currentBootTime) >= eventstore.DefaultRetention {
		log.Logger.Debugw("skipping reboot event", "time_since", time.Since(currentBootTime), "retention", eventstore.DefaultRetention)
		return nil
	}

	curRebootEvent := eventstore.Event{
		Time:    currentBootTime,
		Name:    RebootEventName,
		Type:    string(apiv1.EventTypeWarning),
		Message: fmt.Sprintf("system reboot detected %v", currentBootTime),
	}

	// to prevent duplicate events
	// in case "uptime -s" returns outdated but already recorded timestamp
	existing, err := s.bucket.Find(ctx, curRebootEvent)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	prevRebootEvent, err := s.bucket.Latest(ctx)
	if err != nil {
		return err
	}

	// no previous reboot event
	if prevRebootEvent == nil {
		return s.bucket.Insert(ctx, curRebootEvent)
	}

	// previous reboot event happened after the current reboot event
	// thus do not insert the outdated reboot event
	if !prevRebootEvent.Time.IsZero() && prevRebootEvent.Time.After(currentBootTime) {
		return nil
	}

	// reboot already recorded in the last minute, thus skip
	elapsed := currentBootTime.Sub(prevRebootEvent.Time)
	if elapsed > 0 && elapsed < time.Minute {
		return nil
	}

	// reboot not recorded in the last minute, thus record
	return s.bucket.Insert(ctx, curRebootEvent)
}

func (s *rebootsStore2) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	// we used the same bucket "os" for both reboot and os events
	// until we migrate, we need manual filtering
	// otherwise, we will get non-reboot events from the "os" component kmsg watcher
	return s.bucket.Get(ctx, since, eventstore.WithEventNamesToSelect(RebootEventName))
}
