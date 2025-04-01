// Package sxid tracks the NVIDIA GPU SXid errors scanning the dmesg.
// See fabric manager documentation https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf.
package sxid

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
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
)

const Name = "accelerator-nvidia-error-sxid"

const (
	StateNameErrorSXid = "error_sxid"

	EventNameErroSXid    = "error_sxid"
	EventKeyErroSXidData = "data"
	EventKeyDeviceUUID   = "device_uuid"

	DefaultRetentionPeriod   = eventstore.DefaultRetention
	DefaultStateUpdatePeriod = 30 * time.Second
)

var _ components.Component = &SXIDComponent{}

type SXIDComponent struct {
	rootCtx          context.Context
	cancel           context.CancelFunc
	currState        components.State
	extraEventCh     chan *components.Event
	rebootEventStore pkghost.RebootEventStore
	eventBucket      eventstore.Bucket
	mu               sync.RWMutex

	// experimental
	kmsgWatcher kmsg.Watcher
}

func New(ctx context.Context, rebootEventStore pkghost.RebootEventStore, eventStore eventstore.Store) *SXIDComponent {
	cctx, ccancel := context.WithCancel(ctx)

	extraEventCh := make(chan *components.Event, 256)
	eventBucket, err := eventStore.Bucket(Name)
	if err != nil {
		log.Logger.Errorw("failed to create store", "error", err)
		ccancel()
		return nil
	}

	kmsgWatcher, err := kmsg.StartWatch(func(line string) (eventName string, message string) {
		sxidErr := Match(line)
		if sxidErr == nil {
			return "", ""
		}
		return fmt.Sprintf("SXID %d %s", sxidErr.Detail.SXid, sxidErr.Detail.Name), sxidErr.Detail.Description
	})
	if err != nil {
		ccancel()
		return nil
	}

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

func (c *SXIDComponent) States(ctx context.Context) ([]components.State, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return []components.State{c.currState}, nil
}

func (c *SXIDComponent) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	var ret []components.Event
	events, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		ret = append(ret, resolveSXIDEvent(event))
	}
	return ret, nil
}

func (c *SXIDComponent) Metrics(ctx context.Context, since time.Time) ([]components.Metric, error) {
	log.Logger.Debugw("querying metrics", "since", since)

	return nil, nil
}

func (c *SXIDComponent) Close() error {
	log.Logger.Debugw("closing SXIDComponent")
	c.cancel()

	if c.kmsgWatcher != nil {
		c.kmsgWatcher.Close()
	}

	return nil
}

func (c *SXIDComponent) start(watcher pkg_dmesg.Watcher, updatePeriod time.Duration) {
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
		case dmesgLine := <-watcher.Watch():
			log.Logger.Debugw("dmesg line", "line", dmesgLine)
			sxidErr := Match(dmesgLine.Content)
			if sxidErr == nil {
				log.Logger.Debugw("not xid event, skip")
				continue
			}
			event := components.Event{
				Time: metav1.Time{Time: dmesgLine.Timestamp.Add(time.Duration(rand.Intn(1000)) * time.Millisecond)},
				Name: EventNameErroSXid,
				ExtraInfo: map[string]string{
					EventKeyErroSXidData: strconv.FormatInt(int64(sxidErr.SXid), 10),
					EventKeyDeviceUUID:   sxidErr.DeviceUUID,
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
			if err = c.updateCurrentState(); err != nil {
				log.Logger.Errorw("failed to update current state", "error", err)
				continue
			}
		}
	}
}

func (c *SXIDComponent) SetHealthy() error {
	log.Logger.Debugw("set healthy event received")
	newEvent := &components.Event{Time: metav1.Time{Time: time.Now().UTC()}, Name: "SetHealthy"}
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
