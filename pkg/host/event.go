package host

import (
	"context"
	"fmt"
	"sort"
	"sync"
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
	getLastRebootTime func(context.Context) (time.Time, error)
	eventStore        eventstore.Store

	bucketMu sync.Mutex
	// eventBucket is used for write paths where purge should remain enabled.
	eventBucket eventstore.Bucket
	// eventBucketNoPurge is used for read paths where purge is explicitly disabled.
	eventBucketNoPurge eventstore.Bucket
}

func NewRebootEventStore(eventStore eventstore.Store) RebootEventStore {
	return &rebootEventStore{
		getLastRebootTime: LastReboot,
		eventStore:        eventStore,
	}
}

func (s *rebootEventStore) RecordReboot(ctx context.Context) error {
	return recordEvent(ctx, s, time.Now().UTC(), s.getLastRebootTime)
}

func (s *rebootEventStore) GetRebootEvents(ctx context.Context, since time.Time) (eventstore.Events, error) {
	return getEvents(ctx, s, since)
}

func (s *rebootEventStore) getBucket(disablePurge bool) (eventstore.Bucket, error) {
	s.bucketMu.Lock()
	defer s.bucketMu.Unlock()

	if disablePurge {
		if s.eventBucketNoPurge != nil {
			return s.eventBucketNoPurge, nil
		}
		bucket, err := s.eventStore.Bucket(EventBucketName, eventstore.WithDisablePurge())
		if err != nil {
			return nil, err
		}
		s.eventBucketNoPurge = bucket
		return bucket, nil
	}

	if s.eventBucket != nil {
		return s.eventBucket, nil
	}
	bucket, err := s.eventStore.Bucket(EventBucketName)
	if err != nil {
		return nil, err
	}
	s.eventBucket = bucket
	return bucket, nil
}

func (s *rebootEventStore) Close() error {
	s.bucketMu.Lock()
	defer s.bucketMu.Unlock()

	bucket := s.eventBucket
	bucketNoPurge := s.eventBucketNoPurge
	s.eventBucket = nil
	s.eventBucketNoPurge = nil

	if bucket != nil {
		bucket.Close()
	}
	if bucketNoPurge != nil && bucketNoPurge != bucket {
		bucketNoPurge.Close()
	}
	return nil
}

func recordEvent(ctx context.Context, store *rebootEventStore, now time.Time, getLastRebootTime func(context.Context) (time.Time, error)) error {
	currentBootTime, err := getLastRebootTime(ctx)
	if err != nil {
		return err
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

	bucket, err := store.getBucket(false)
	if err != nil {
		return err
	}

	// to prevent duplicate events
	// in case "uptime -s" returns outdated but already recorded timestamp
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

func getEvents(ctx context.Context, store *rebootEventStore, since time.Time) (eventstore.Events, error) {
	rebootBucket, err := store.getBucket(true)
	if err != nil {
		return nil, err
	}
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
