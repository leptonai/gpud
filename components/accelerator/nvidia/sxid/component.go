// Package sxid tracks the NVIDIA GPU SXid errors scanning the kmsg.
// See fabric manager documentation https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf.
package sxid

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

const Name = "accelerator-nvidia-error-sxid"

const (
	StateNameErrorSXid = "error_sxid"

	EventNameErrorSXid    = "error_sxid"
	EventKeyErrorSXidData = "data"
	EventKeyDeviceUUID    = "device_uuid"

	DefaultRetentionPeriod   = eventstore.DefaultRetention
	DefaultStateUpdatePeriod = 30 * time.Second
)

var _ components.Component = &SXIDComponent{}

type SXIDComponent struct {
	rootCtx          context.Context
	cancel           context.CancelFunc
	extraEventCh     chan *apiv1.Event
	rebootEventStore pkghost.RebootEventStore
	eventBucket      eventstore.Bucket
	kmsgWatcher      kmsg.Watcher
	mu               sync.RWMutex
	currState        apiv1.HealthState
}

func New(ctx context.Context, rebootEventStore pkghost.RebootEventStore, eventStore eventstore.Store) *SXIDComponent {
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		log.Logger.Errorw("failed to create store", "error", err)
		return nil
	}

	kmsgWatcher, err := kmsg.NewWatcher()
	if err != nil {
		log.Logger.Errorw("failed to create kmsg watcher", "error", err)
		return nil
	}

	cctx, ccancel := context.WithCancel(ctx)
	extraEventCh := make(chan *apiv1.Event, 256)
	return &SXIDComponent{
		rootCtx:          cctx,
		cancel:           ccancel,
		extraEventCh:     extraEventCh,
		rebootEventStore: rebootEventStore,
		eventBucket:      eventBucket,
		kmsgWatcher:      kmsgWatcher,
	}
}

func (c *SXIDComponent) Name() string { return Name }

func (c *SXIDComponent) Start() error {
	for {
		err := c.updateCurrentState()
		if err == nil {
			break
		}
		if errors.Is(err, context.Canceled) {
			log.Logger.Infow("context canceled, exiting")
			return nil
		}
		log.Logger.Errorw("failed to fetch current events", "error", err)
		select {
		case <-c.rootCtx.Done():
			return nil
		case <-time.After(1 * time.Second):
		}
	}

	kmsgCh, err := c.kmsgWatcher.Watch()
	if err != nil {
		return err
	}
	go c.start(kmsgCh, DefaultStateUpdatePeriod)

	return nil
}

func (c *SXIDComponent) HealthStates(ctx context.Context) (apiv1.HealthStates, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return apiv1.HealthStates{c.currState}, nil
}

func (c *SXIDComponent) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	var ret apiv1.Events
	events, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		ret = append(ret, resolveSXIDEvent(event))
	}
	return ret, nil
}

func (c *SXIDComponent) Close() error {
	log.Logger.Debugw("closing SXIDComponent")
	c.cancel()

	if c.kmsgWatcher != nil {
		_ = c.kmsgWatcher.Close()
	}

	return nil
}

func (c *SXIDComponent) start(kmsgCh <-chan kmsg.Message, updatePeriod time.Duration) {
	ticker := time.NewTicker(updatePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-c.rootCtx.Done():
			return
		case <-ticker.C:
			if err := c.updateCurrentState(); err != nil {
				log.Logger.Debugw("failed to fetch current events", "error", err)
				continue
			}
		case newEvent := <-c.extraEventCh:
			if newEvent == nil {
				continue
			}
			if err := c.eventBucket.Insert(c.rootCtx, *newEvent); err != nil {
				log.Logger.Errorw("failed to create event", "error", err)
				continue
			}
			if err := c.updateCurrentState(); err != nil {
				log.Logger.Errorw("failed to update current state", "error", err)
				continue
			}
		case message := <-kmsgCh:
			sxidErr := Match(message.Message)
			if sxidErr == nil {
				log.Logger.Debugw("not sxid event, skip", "kmsg", message)
				continue
			}

			id := uuid.New()
			var sxidName string
			if sxidErr.Detail != nil {
				sxidName = sxidErr.Detail.Name
			}
			logger := log.Logger.With("id", id, "sxid", sxidErr.SXid, "sxidName", sxidName, "deviceUUID", sxidErr.DeviceUUID)
			logger.Infow("got sxid event", "kmsg", message, "kmsgTimestamp", message.Timestamp.Unix())

			event := apiv1.Event{
				Time: message.Timestamp,
				Name: EventNameErrorSXid,
				DeprecatedExtraInfo: map[string]string{
					EventKeyErrorSXidData: strconv.FormatInt(int64(sxidErr.SXid), 10),
					EventKeyDeviceUUID:    sxidErr.DeviceUUID,
				},
			}
			sameEvent, err := c.eventBucket.Find(c.rootCtx, event)
			if err != nil {
				logger.Errorw("failed to check event existence", "error", err)
				continue
			}
			if sameEvent != nil {
				logger.Infow("find the same event, skip inserting it")
				continue
			}
			if err = c.eventBucket.Insert(c.rootCtx, event); err != nil {
				logger.Errorw("failed to create event", "error", err)
				continue
			}
			logger.Infow("inserted the event successfully")
			if err = c.updateCurrentState(); err != nil {
				logger.Errorw("failed to update current state", "error", err)
				continue
			}
		}
	}
}

var _ components.HealthSettable = &SXIDComponent{}

func (c *SXIDComponent) SetHealthy() error {
	log.Logger.Debugw("set healthy event received")
	newEvent := &apiv1.Event{Time: metav1.Time{Time: time.Now().UTC()}, Name: "SetHealthy"}
	select {
	case c.extraEventCh <- newEvent:
	default:
		log.Logger.Debugw("channel full, set healthy event skipped")
	}
	return nil
}

func (c *SXIDComponent) updateCurrentState() error {
	var rebootErr string
	rebootEvents, err := c.rebootEventStore.GetRebootEvents(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
	if err != nil {
		rebootErr = fmt.Sprintf("failed to get reboot events: %v", err)
		log.Logger.Errorw("failed to get reboot events", "error", err)
	}
	localEvents, err := c.eventBucket.Get(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
	if err != nil {
		return fmt.Errorf("failed to get all events: %w", err)
	}
	events := mergeEvents(rebootEvents, localEvents)
	c.mu.Lock()
	c.currState = EvolveHealthyState(events)
	if rebootErr != "" {
		c.currState.Error = fmt.Sprintf("%s\n%s", rebootErr, c.currState.Error)
	}
	c.mu.Unlock()
	return nil
}

// mergeEvents merges two event slices and returns a time descending sorted new slice
func mergeEvents(a, b apiv1.Events) apiv1.Events {
	totalLen := len(a) + len(b)
	if totalLen == 0 {
		return nil
	}
	result := make(apiv1.Events, 0, totalLen)
	result = append(result, a...)
	result = append(result, b...)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Time.After(result[j].Time.Time)
	})

	return result
}
