package reboot

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
)

func RecordEvent(ctx context.Context, eventStore eventstore.Store, bucketName string, lastRebootTime func(context.Context) (time.Time, error)) error {
	bootTime, err := lastRebootTime(ctx)
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
		Type:    common.EventTypeWarning,
		Message: fmt.Sprintf("system reboot detected %v", bootTime),
	}

	bucket, err := eventStore.LoadBucketWithNoPurge(bucketName)
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

func GetEvents(ctx context.Context, eventStore eventstore.Store, bucketName string, since time.Time) ([]components.Event, error) {
	bucket, err := eventStore.LoadBucketWithNoPurge(bucketName)
	if err != nil {
		return nil, err
	}
	defer bucket.Close()

	return bucket.Get(ctx, since)
}
