// Package xid tracks the NVIDIA GPU Xid errors scanning the dmesg
// and using the NVIDIA Management Library (NVML).
// See Xid messages https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages.
package xid

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
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	"github.com/leptonai/gpud/components/accelerator/nvidia/query/xid/dmesg"
	"github.com/leptonai/gpud/components/db"
	os_id "github.com/leptonai/gpud/components/os/id"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
)

const (
	DefaultRetentionPeriod   = 3 * 24 * time.Hour
	DefaultStateUpdatePeriod = 30 * time.Second
)

type XIDComponent struct {
	rootCtx      context.Context
	cancel       context.CancelFunc
	poller       query.Poller
	currState    components.State
	extraEventCh chan *components.Event
	store        db.Store
	mu           sync.RWMutex
}

func New(ctx context.Context, cfg nvidia_common.Config, dbRW *sql.DB, dbRO *sql.DB) *XIDComponent {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, nvidia_component_error_xid_id.Name)

	extraEventCh := make(chan *components.Event, 256)
	localStore, err := db.NewStore(dbRW, dbRO, db.CreateDefaultTableName(nvidia_component_error_xid_id.Name), DefaultRetentionPeriod)
	if err != nil {
		log.Logger.Errorw("failed to create store", "error", err)
		ccancel()
		return nil
	}
	return &XIDComponent{
		rootCtx:      cctx,
		cancel:       ccancel,
		poller:       getDefaultPoller(),
		extraEventCh: extraEventCh,
		store:        localStore,
	}
}

var _ components.Component = (*XIDComponent)(nil)

func (c *XIDComponent) Name() string { return nvidia_component_error_xid_id.Name }

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
	events, err := c.store.Get(ctx, since)
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
	c.poller.Stop(nvidia_component_error_xid_id.Name)
	c.cancel()
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
			if err := c.store.Insert(c.rootCtx, *newEvent); err != nil {
				log.Logger.Errorw("failed to create event", "error", err)
				continue
			}
			events, err := c.store.Get(c.rootCtx, time.Time{})
			if err != nil {
				log.Logger.Errorw("failed to get all events", "error", err)
				continue
			}
			c.mu.Lock()
			c.currState = EvolveHealthyState(events)
			c.mu.Unlock()
		case dmesgLine := <-watcher.Watch():
			log.Logger.Debugw("dmesg line", "line", dmesgLine)
			xidErr := dmesg.Match(dmesgLine.Content)
			if xidErr == nil {
				log.Logger.Debugw("not xid event, skip")
				continue
			}
			event := components.Event{
				Time: metav1.Time{Time: dmesgLine.Timestamp},
				Name: EventNameErroXid,
				ExtraInfo: map[string]string{
					EventKeyErroXidData: strconv.FormatInt(int64(xidErr.Xid), 10),
					EventKeyDeviceUUID:  xidErr.DeviceUUID,
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
			events, err := c.store.Get(c.rootCtx, time.Time{})
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
	osComponent, err := components.GetComponent(os_id.Name)
	if err != nil {
		return fmt.Errorf("failed to get os component: %w", err)
	}
	osEvents, err := osComponent.Events(c.rootCtx, time.Now().Add(-DefaultRetentionPeriod))
	if err != nil {
		return fmt.Errorf("failed to get os events: %w", err)
	}
	localEvents, err := c.store.Get(c.rootCtx, time.Time{})
	if err != nil {
		return fmt.Errorf("failed to get all events: %w", err)
	}
	events := mergeEvents(osEvents, localEvents)
	c.mu.Lock()
	c.currState = EvolveHealthyState(events)
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
