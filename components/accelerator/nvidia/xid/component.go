// Package xid tracks the NVIDIA GPU Xid errors scanning the dmesg
// and using the NVIDIA Management Library (NVML).
// See Xid messages https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages.
package xid

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	os_id "github.com/leptonai/gpud/components/os/id"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

const Name = "accelerator-nvidia-error-xid"

const (
	StateNameErrorXid = "error_xid"

	EventNameErrorXid    = "error_xid"
	EventKeyErrorXidData = "data"
	EventKeyDeviceUUID   = "device_uuid"

	DefaultRetentionPeriod   = eventstore.DefaultRetention
	DefaultStateUpdatePeriod = 30 * time.Second
)

type XIDComponent struct {
	rootCtx      context.Context
	cancel       context.CancelFunc
	currState    components.State
	extraEventCh chan *components.Event
	eventBucket  eventstore.Bucket
	mu           sync.RWMutex

	// experimental
	kmsgWatcher kmsg.Watcher
}

func New(ctx context.Context, eventStore eventstore.Store) *XIDComponent {
	cctx, ccancel := context.WithCancel(ctx)

	extraEventCh := make(chan *components.Event, 256)
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		log.Logger.Errorw("failed to create store", "error", err)
		ccancel()
		return nil
	}

	kmsgWatcher, err := kmsg.StartWatch(func(line string) (eventName string, message string) {
		xidErr := Match(line)
		if xidErr == nil {
			return "", ""
		}
		return fmt.Sprintf("XID %d %s", xidErr.Detail.Xid, xidErr.Detail.Name), xidErr.Detail.Description
	})
	if err != nil {
		ccancel()
		return nil
	}

	return &XIDComponent{
		rootCtx:      cctx,
		cancel:       ccancel,
		extraEventCh: extraEventCh,
		eventBucket:  eventBucket,
		kmsgWatcher:  kmsgWatcher,
	}
}

var _ components.Component = (*XIDComponent)(nil)

func (c *XIDComponent) Name() string { return Name }

func (c *XIDComponent) Start() error {
	initializeBackoff := 1 * time.Second
	for {
		if err := c.updateCurrentState(); err != nil {
			if strings.Contains(err.Error(), context.Canceled.Error()) {
				log.Logger.Infow("context canceled, exiting")
				return nil
			}
			log.Logger.Errorw("failed to fetch current events", "error", err)
			time.Sleep(initializeBackoff)
			continue
		}
		break
	}
	watcher, err := pkg_dmesg.NewWatcher()
	if err != nil {
		log.Logger.Errorw("failed to create dmesg watcher", "error", err)
		return nil
	}

	go c.start(watcher, DefaultStateUpdatePeriod)

	return nil
}

func (c *XIDComponent) States(_ context.Context) ([]components.State, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return []components.State{c.currState}, nil
}

func (c *XIDComponent) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	var ret []components.Event
	events, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		ret = append(ret, resolveXIDEvent(event))
	}
	return ret, nil
}

func (c *XIDComponent) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *XIDComponent) Close() error {
	log.Logger.Debugw("closing XIDComponent")
	// safe to call stop multiple times
	c.cancel()

	if c.kmsgWatcher != nil {
		c.kmsgWatcher.Close()
	}

	return nil
}

func (c *XIDComponent) start(watcher pkg_dmesg.Watcher, updatePeriod time.Duration) {
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
			events, err := c.eventBucket.Get(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
			if err != nil {
				log.Logger.Errorw("failed to get all events", "error", err)
				continue
			}
			c.mu.Lock()
			c.currState = EvolveHealthyState(events)
			c.mu.Unlock()

		case dmesgLine := <-watcher.Watch():
			log.Logger.Debugw("dmesg line", "line", dmesgLine)
			xidErr := Match(dmesgLine.Content)
			if xidErr == nil {
				log.Logger.Debugw("not xid event, skip")
				continue
			}
			event := components.Event{
				Time: metav1.Time{Time: dmesgLine.Timestamp.Add(time.Duration(rand.Intn(1000)) * time.Millisecond)},
				Name: EventNameErrorXid,
				ExtraInfo: map[string]string{
					EventKeyErrorXidData: strconv.FormatInt(int64(xidErr.Xid), 10),
					EventKeyDeviceUUID:   xidErr.DeviceUUID,
				},
			}
			currEvent, err := c.eventBucket.Find(c.rootCtx, event)
			if err != nil {
				log.Logger.Errorw("failed to check event existence", "error", err)
				continue
			}

			if currEvent != nil {
				log.Logger.Debugw("no new events created")
				continue
			}
			if err = c.eventBucket.Insert(c.rootCtx, event); err != nil {
				log.Logger.Errorw("failed to create event", "error", err)
				continue
			}
			events, err := c.eventBucket.Get(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
			if err != nil {
				log.Logger.Errorw("failed to get all events", "error", err)
				continue
			}
			c.mu.Lock()
			c.currState = EvolveHealthyState(events)
			c.mu.Unlock()
		}
	}
}

func (c *XIDComponent) SetHealthy() error {
	log.Logger.Debugw("set healthy event received")
	newEvent := &components.Event{Time: metav1.Time{Time: time.Now().UTC()}, Name: "SetHealthy"}
	select {
	case c.extraEventCh <- newEvent:
	default:
		log.Logger.Debugw("channel full, set healthy event skipped")
	}
	return nil
}

func (c *XIDComponent) updateCurrentState() error {
	rebootEvents, err := getRebootEvents(c.rootCtx)
	if err != nil {
		return fmt.Errorf("failed to get reboot events: %w", err)
	}
	localEvents, err := c.eventBucket.Get(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
	if err != nil {
		return fmt.Errorf("failed to get all events: %w", err)
	}
	events := mergeEvents(rebootEvents, localEvents)
	c.mu.Lock()
	c.currState = EvolveHealthyState(events)
	c.mu.Unlock()
	return nil
}

func getRebootEvents(ctx context.Context) ([]components.Event, error) {
	osComponent, err := components.GetComponent(os_id.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get os component: %w", err)
	}
	osEvents, err := osComponent.Events(ctx, time.Now().Add(-DefaultRetentionPeriod))
	if err != nil {
		return nil, fmt.Errorf("failed to get os events: %w", err)
	}
	return osEvents, nil
}

// mergeEvents merges two event slices and returns a time descending sorted new slice
func mergeEvents(a, b []components.Event) []components.Event {
	totalLen := len(a) + len(b)
	if totalLen == 0 {
		return nil
	}
	result := make([]components.Event, 0, totalLen)
	result = append(result, a...)
	result = append(result, b...)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Time.After(result[j].Time.Time)
	})

	return result
}
