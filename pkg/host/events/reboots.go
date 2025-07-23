package events

import (
	"context"
	"fmt"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	// RebootBucketName is the bucket name for the reboot events.
	// For historical reasons, we use the same bucket name as the "os" component.
	RebootBucketName = "os"

	// RebootEventName is the event name for the reboot events.
	RebootEventName = "reboot"
)

// RebootsStore is the interface for the reboot event store.
// It is used to record and query reboot events.
type RebootsStore interface {
	// Record records a reboot event, with the event name [RebootEventName],
	// in the bucket [RebootBucketName].
	Record(ctx context.Context) error

	// Get queries all "reboot" events and if any extra buckets are provided,
	// it will also query the events from the extra buckets, with the same since time.
	// The returned events do NOT include other events from the "os" component (e.g., kmsg watcher).
	// The returned events are in the descending order of timestamp (latest event first).
	Get(ctx context.Context, since time.Time) (eventstore.Events, error)
}

var _ RebootsStore = &rebootsStore{}

type rebootsStore struct {
	getTimeNowFunc    func() time.Time
	getLastRebootTime func(context.Context) (time.Time, error)
	bucket            eventstore.Bucket
}

func NewRebootsStore(bucket eventstore.Bucket) RebootsStore {
	return &rebootsStore{
		getTimeNowFunc:    func() time.Time { return time.Now().UTC() },
		getLastRebootTime: host.LastReboot,
		bucket:            bucket,
	}
}

func (s *rebootsStore) Record(ctx context.Context) error {
	currentBootTime, err := s.getLastRebootTime(ctx)
	if err != nil {
		return err
	}

	// if now - event time > retention (too old), then skip
	now := s.getTimeNowFunc()
	elapsedSinceLastReboot := now.Sub(currentBootTime)
	if elapsedSinceLastReboot >= eventstore.DefaultRetention {
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
	rebootDelta := currentBootTime.Sub(prevRebootEvent.Time)
	if rebootDelta > 0 && rebootDelta < time.Minute {
		return nil
	}

	// reboot not recorded in the last minute, thus record
	return s.bucket.Insert(ctx, curRebootEvent)
}

func (s *rebootsStore) Get(ctx context.Context, since time.Time) (eventstore.Events, error) {
	// we used the same bucket "os" for both reboot and os events
	// until we migrate, we need manual filtering
	// otherwise, we will get non-reboot events from the "os" component kmsg watcher
	evs, err := s.bucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}

	// filter out non-reboot events
	// TODO: we should just use "WHERE" statement
	rebootEvs := make(eventstore.Events, 0, len(evs))
	for _, ev := range evs {
		if ev.Name == RebootEventName {
			rebootEvs = append(rebootEvs, ev)
		}
	}

	return rebootEvs, nil
}
