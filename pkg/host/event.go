package host

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	EventBucketName = "os"
	EventNameReboot = "reboot"
)

// RebootEventStore is the interface for the reboot event store.
// It is used to record and query reboot events.
type RebootEventStore interface {
	// RecordReboot records a reboot event, with the event name [EventNameReboot],
	// in the bucket [EventBucketName].
	RecordReboot(ctx context.Context) error

	// GetRebootEvents queries all "reboot" events and if any extra buckets are provided,
	// it will also query the events from the extra buckets, with the same since time.
	// The returned events do NOT include other events from the "os" component (e.g., kmsg watcher).
	// The returned events are in the descending order of timestamp (latest event first).
	GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error)
}

var _ RebootEventStore = &rebootEventStore{}

type rebootEventStore struct {
	getBootTime func() time.Time
	eventStore  eventstore.Store
}

func NewRebootEventStore(eventStore eventstore.Store) RebootEventStore {
	return &rebootEventStore{
		getBootTime: BootTime,
		eventStore:  eventStore,
	}
}

func (s *rebootEventStore) RecordReboot(ctx context.Context) error {
	return recordEvent(ctx, s.eventStore, time.Now().UTC(), s.getBootTime)
}

func (s *rebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return getEvents(ctx, s.eventStore, since)
}

var ErrBootTimeUnavailable = errors.New("boot time unavailable")

func recordEvent(ctx context.Context, rebootEventStore eventstore.Store, now time.Time, getBootTime func() time.Time) error {
	currentBootTime := getBootTime()
	if currentBootTime.IsZero() {
		return ErrBootTimeUnavailable
	}

	// if now - event time > retention, then skip
	if now.Sub(currentBootTime) >= eventstore.DefaultRetention {
		log.Logger.Debugw("skipping reboot event", "time_since", time.Since(currentBootTime), "retention", eventstore.DefaultRetention)
		return nil
	}

	curRebootEvent := eventstore.Event{
		Time:    currentBootTime,
		Name:    EventNameReboot,
		Type:    string(apiv1.EventTypeWarning),
		Message: fmt.Sprintf("system reboot detected %v", currentBootTime),
	}

	bucket, err := rebootEventStore.Bucket(EventBucketName)
	if err != nil {
		return err
	}
	defer bucket.Close()

	// to prevent duplicate events
	// in case boot time syscall returns an already recorded timestamp
	existing, err := bucket.Find(ctx, curRebootEvent)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	prevRebootEvent, err := bucket.Latest(ctx)
	if err != nil {
		return err
	}

	// no previous reboot event
	if prevRebootEvent == nil {
		return bucket.Insert(ctx, curRebootEvent)
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
	return bucket.Insert(ctx, curRebootEvent)
}

func getEvents(ctx context.Context, eventStore eventstore.Store, since time.Time) (eventstore.Events, error) {
	rebootBucket, err := eventStore.Bucket(EventBucketName, eventstore.WithDisablePurge())
	if err != nil {
		return nil, err
	}
	defer rebootBucket.Close()

	// we used the same bucket "os" for both reboot and os events
	// until we migrate, we need manual filtering
	// otherwise, we will get non-reboot events from the "os" component kmsg watcher
	allOSEvents, err := rebootBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}

	all := make(eventstore.Events, 0, len(allOSEvents)/2)
	for _, ev := range allOSEvents {
		// The returned events should NOT include other events from the "os" component (e.g., kmsg watcher).
		if ev.Name != EventNameReboot {
			continue
		}
		all = append(all, ev)
	}

	// The returned events are in the descending order of timestamp (latest event first).
	sort.Slice(all, func(i, j int) bool {
		return all[i].Time.After(all[j].Time)
	})

	return all, nil
}
