package host

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

const defaultBucketName = "os"

type RebootEventStore interface {
	RecordReboot(ctx context.Context) error
	GetRebootEvents(ctx context.Context, since time.Time) ([]components.Event, error)
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
	return recordEvent(ctx, s.eventStore, s.getLastRebootTime)
}

func (s *rebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) ([]components.Event, error) {
	return getEvents(ctx, s.eventStore, since)
}

func recordEvent(ctx context.Context, eventStore eventstore.Store, getLastRebootTime func(context.Context) (time.Time, error)) error {
	bootTime, err := getLastRebootTime(ctx)
	if err != nil {
		return err
	}

	// if now - event time > retention, then skip
	if time.Since(bootTime) >= eventstore.DefaultRetention {
		log.Logger.Debugw("skipping reboot event", "time_since", time.Since(bootTime), "retention", eventstore.DefaultRetention)
		return nil
	}

	rebootEvent := components.Event{
		Time:    metav1.Time{Time: bootTime},
		Name:    "reboot",
		Type:    components.EventTypeWarning,
		Message: fmt.Sprintf("system reboot detected %v", bootTime),
	}

	bucket, err := eventStore.Bucket(defaultBucketName, eventstore.WithDisablePurge())
	if err != nil {
		return err
	}
	defer bucket.Close()

	lastEvent, err := bucket.Latest(ctx)
	if err != nil {
		return err
	}

	if lastEvent != nil && lastEvent.Time.Time.Sub(bootTime).Abs() < time.Minute {
		return nil
	}

	// else insert event
	return bucket.Insert(ctx, rebootEvent)
}

func getEvents(ctx context.Context, eventStore eventstore.Store, since time.Time) ([]components.Event, error) {
	bucket, err := eventStore.Bucket(defaultBucketName, eventstore.WithDisablePurge())
	if err != nil {
		return nil, err
	}
	defer bucket.Close()

	return bucket.Get(ctx, since)
}
