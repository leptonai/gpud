// Package sxid tracks the NVIDIA GPU SXid errors scanning the dmesg.
// See fabric manager documentation https://docs.nvidia.com/datacenter/tesla/pdf/fabric-manager-user-guide.pdf.
package sxid

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	nvidia_component_error_sxid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid/id"
	os_id "github.com/leptonai/gpud/components/os/id"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
	events_db "github.com/leptonai/gpud/pkg/events-db"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	StateNameErrorSXid = "error_sxid"

	EventNameErroSXid    = "error_sxid"
	EventKeyErroSXidData = "data"
	EventKeyDeviceUUID   = "device_uuid"

	DefaultRetentionPeriod   = 3 * 24 * time.Hour
	DefaultStateUpdatePeriod = 30 * time.Second
)

type SXIDComponent struct {
	rootCtx      context.Context
	cancel       context.CancelFunc
	currState    components.State
	extraEventCh chan *components.Event
	store        events_db.Store
	mu           sync.RWMutex
}

func New(ctx context.Context, dbRW *sql.DB, dbRO *sql.DB) *SXIDComponent {
	cctx, ccancel := context.WithCancel(ctx)

	extraEventCh := make(chan *components.Event, 256)
	localStore, err := events_db.NewStore(dbRW, dbRO, events_db.CreateDefaultTableName(nvidia_component_error_sxid_id.Name), DefaultRetentionPeriod)
	if err != nil {
		log.Logger.Errorw("failed to create store", "error", err)
		ccancel()
		return nil
	}
	return &SXIDComponent{
		rootCtx:      cctx,
		cancel:       ccancel,
		extraEventCh: extraEventCh,
		store:        localStore,
	}
}

var _ components.Component = (*SXIDComponent)(nil)

func (c *SXIDComponent) Name() string { return nvidia_component_error_sxid_id.Name }

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
	events, err := c.store.Get(ctx, since)
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
			if err := c.store.Insert(c.rootCtx, *newEvent); err != nil {
				log.Logger.Errorw("failed to create event", "error", err)
				continue
			}
			events, err := c.store.Get(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
			if err != nil {
				log.Logger.Errorw("failed to get all events", "error", err)
				continue
			}
			c.mu.Lock()
			c.currState = EvolveHealthyState(events)
			c.mu.Unlock()
		case dmesgLine := <-watcher.Watch():
			log.Logger.Debugw("dmesg line", "line", dmesgLine)
			sxidErr := Match(dmesgLine.Content)
			if sxidErr == nil {
				log.Logger.Debugw("not xid event, skip")
				continue
			}
			event := components.Event{
				Time: metav1.Time{Time: dmesgLine.Timestamp},
				Name: EventNameErroSXid,
				ExtraInfo: map[string]string{
					EventKeyErroSXidData: strconv.FormatInt(int64(sxidErr.SXid), 10),
					EventKeyDeviceUUID:   sxidErr.DeviceUUID,
				},
			}
			currEvent, err := c.store.Find(c.rootCtx, event)
			if err != nil {
				log.Logger.Errorw("failed to check event existence", "error", err)
				continue
			}

			if currEvent != nil {
				log.Logger.Debugw("no new events created")
				continue
			}
			if err = c.store.Insert(c.rootCtx, event); err != nil {
				log.Logger.Errorw("failed to create event", "error", err)
				continue
			}
			events, err := c.store.Get(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
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
	rebootEvents, err := getRebootEvents(c.rootCtx)
	if err != nil {
		return fmt.Errorf("failed to get reboot events: %w", err)
	}
	localEvents, err := c.store.Get(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
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
