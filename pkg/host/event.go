package host

import (
	"context"
	"fmt"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

const defaultBucketName = "os"

type RebootEventStore interface {
	RecordReboot(ctx context.Context) error
	GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error)
	// PurgeAll purges all reboot events in the table.
	PurgeAll(ctx context.Context) error
}

var _ RebootEventStore = &rebootEventStore{}

type rebootEventStore struct {
	getLastRebootTime func(context.Context) (time.Time, error)
	eventStore        eventstore.Store
}

func NewRebootEventStore(eventStore eventstore.Store) RebootEventStore {
	return &rebootEventStore{
		getLastRebootTime: LastReboot,
		eventStore:        eventStore,
	}
}

func (s *rebootEventStore) RecordReboot(ctx context.Context) error {
	return recordEvent(ctx, s.eventStore, time.Now().UTC(), s.getLastRebootTime)
}

func (s *rebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return getEvents(ctx, s.eventStore, since)
}

func (s *rebootEventStore) PurgeAll(ctx context.Context) error {
	bucket, err := s.eventStore.Bucket(defaultBucketName, eventstore.WithDisablePurge())
	if err != nil {
		return err
	}
	defer bucket.Close()

	now := time.Now().UTC()
	now = now.Add(24 * time.Hour)

	purged, err := bucket.Purge(ctx, now.Unix())
	if err != nil {
		return err
	}
	log.Logger.Infow("purged reboot events", "purged", purged)

	return nil
}

func recordEvent(ctx context.Context, eventStore eventstore.Store, now time.Time, getLastRebootTime func(context.Context) (time.Time, error)) error {
	currentBootTime, err := getLastRebootTime(ctx)
	if err != nil {
		return err
	}

	// if now - event time > retention, then skip
	if now.Sub(currentBootTime) >= eventstore.DefaultRetention {
		log.Logger.Debugw("skipping reboot event", "time_since", time.Since(currentBootTime), "retention", eventstore.DefaultRetention)
		return nil
	}

	currentRebootEvent := eventstore.Event{
		Time:    currentBootTime,
		Name:    "reboot",
		Type:    string(apiv1.EventTypeWarning),
		Message: fmt.Sprintf("system reboot detected %v", currentBootTime),
	}

	bucket, err := eventStore.Bucket(defaultBucketName, eventstore.WithDisablePurge())
	if err != nil {
		return err
	}
	defer bucket.Close()

	// to prevent duplicate events
	// in case "uptime -s" returns outdated but already recorded timestamp
	existing, err := bucket.Find(ctx, currentRebootEvent)
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
		return bucket.Insert(ctx, currentRebootEvent)
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
	return bucket.Insert(ctx, currentRebootEvent)
}

func getEvents(ctx context.Context, eventStore eventstore.Store, since time.Time) (eventstore.Events, error) {
	bucket, err := eventStore.Bucket(defaultBucketName, eventstore.WithDisablePurge())
	if err != nil {
		return nil, err
	}
	defer bucket.Close()

	return bucket.Get(ctx, since)
}
