// Package xid tracks the NVIDIA GPU Xid errors scanning the dmesg
// and using the NVIDIA Management Library (NVML).
// See Xid messages https://docs.nvidia.com/deploy/gpu-debug-guidelines/index.html#xid-messages.
package xid

import (
	"context"
	"database/sql"
	"sort"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	nvidia_component_error_xid_id "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/id"
	"github.com/leptonai/gpud/components/accelerator/nvidia/error/xid/store"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	os_id "github.com/leptonai/gpud/components/os/id"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
	pkg_dmesg "github.com/leptonai/gpud/pkg/dmesg"
)

func New(ctx context.Context, cfg nvidia_common.Config, db *sql.DB) components.Component {
	cfg.Query.SetDefaultsIfNotSet()
	setDefaultPoller(cfg)

	cctx, ccancel := context.WithCancel(ctx)
	getDefaultPoller().Start(cctx, cfg.Query, nvidia_component_error_xid_id.Name)

	setHealthyCh := make(chan struct{})
	localStore, err := store.New(ctx, db, "components_accelerator_nvidia_error_xid_events")
	if err != nil {
		log.Logger.Errorw("failed to create store", "error", err)
		ccancel()
		return nil
	}
	return &XIDComponent{
		rootCtx:      ctx,
		cancel:       ccancel,
		poller:       getDefaultPoller(),
		setHealthyCh: setHealthyCh,
		store:        localStore,
	}
}

func (c *XIDComponent) SetHealthy() error {
	c.setHealthyCh <- struct{}{}
	return nil
}

var _ components.Component = (*XIDComponent)(nil)

type XIDComponent struct {
	rootCtx      context.Context
	cancel       context.CancelFunc
	poller       query.Poller
	currState    components.State
	setHealthyCh chan struct{}
	store        *store.Store
}

func (c *XIDComponent) Name() string { return nvidia_component_error_xid_id.Name }

func (c *XIDComponent) Start() error {
	watcher, err := pkg_dmesg.NewWatcher()
	if err != nil {
		log.Logger.Errorw("failed to create dmesg watcher", "error", err)
		return nil
	}
	for {
		osComponent, err := components.GetComponent(os_id.Name)
		if err != nil {
			log.Logger.Errorw("failed to get os component", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}
		osEvents, err := osComponent.Events(c.rootCtx, time.Now().Add(-3*24*time.Hour))
		if err != nil {
			log.Logger.Errorw("failed to get os states", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}
		if len(osEvents) < 1 {
			log.Logger.Debugw("no os states found")
			time.Sleep(1 * time.Second)
			continue
		}
		if !osEvents[0].Time.After(time.Now().Add(-3 * 24 * time.Hour)) {
			log.Logger.Debugw("newest reboot event not caught")
			time.Sleep(1 * time.Second)
			continue
		}
		localEvents, err := c.store.GetAllEvents(c.rootCtx)
		if err != nil {
			log.Logger.Errorw("failed to get all events", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}
		events := mergeEvents(osEvents, localEvents)
		c.currState = EvolveHealthyState(events)
		break
	}
	go func() {
		for {
			select {
			case <-c.rootCtx.Done():
				return
			case <-c.setHealthyCh:
				count, err := c.store.CreateEvent(c.rootCtx, components.Event{Time: metav1.Time{Time: time.Now().UTC()}, Name: "SetHealthy"})
				if err != nil {
					log.Logger.Errorw("failed to create event", "error", err)
					continue
				} else if count == 0 {
					log.Logger.Debugw("no new events created")
					continue
				}
				events, err := c.store.GetAllEvents(c.rootCtx)
				if err != nil {
					log.Logger.Errorw("failed to get all events", "error", err)
					continue
				}
				c.currState = EvolveHealthyState(events)
			case dmesgLine := <-watcher.Watch():
				log.Logger.Debugw("dmesg line", "line", dmesgLine)
				ev, err := nvidia_query_xid.ParseDmesgLogLine(metav1.Time{Time: dmesgLine.Timestamp}, dmesgLine.Content)
				if err != nil {
					log.Logger.Errorw("failed to parse dmesg line", "error", err)
					continue
				}
				if ev.Detail == nil {
					log.Logger.Debugw("not xid event, skip")
					continue
				}
				event := components.Event{
					Time: metav1.Time{Time: dmesgLine.Timestamp},
					Name: EventNameErroXid,
					ExtraInfo: map[string]string{
						EventKeyErroXidData: strconv.FormatInt(int64(ev.Detail.Xid), 10),
						EventKeyDeviceUUID:  ev.DeviceUUID,
					},
				}
				count, err := c.store.CreateEvent(c.rootCtx, event)
				if err != nil {
					log.Logger.Errorw("failed to create event", "error", err)
					continue
				} else if count == 0 {
					log.Logger.Debugw("no new events created")
					continue
				}
				events, err := c.store.GetAllEvents(c.rootCtx)
				if err != nil {
					log.Logger.Errorw("failed to get all events", "error", err)
					continue
				}
				c.currState = EvolveHealthyState(events)
			}
		}
	}()
	return nil
}

func (c *XIDComponent) States(_ context.Context) ([]components.State, error) {
	return []components.State{c.currState}, nil
}

func (c *XIDComponent) Events(ctx context.Context, since time.Time) ([]components.Event, error) {
	var ret []components.Event
	events, err := c.store.GetEvents(ctx, since)
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

// mergeEvents merges two event slices and returns a time ascending sorted new slice
func mergeEvents(a, b []components.Event) []components.Event {
	totalLen := len(a) + len(b)
	if totalLen == 0 {
		return nil
	}
	result := make([]components.Event, 0, totalLen)
	result = append(result, a...)
	result = append(result, b...)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Time.Before(result[j].Time.Time)
	})

	return result
}
