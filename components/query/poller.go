package query

import (
	"context"
	"sync"
	"time"

	query_config "github.com/leptonai/gpud/components/query/config"
	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Defines the common query/poller interface.
// It polls the data source (rather than watch) in order
// to share the same data source with multiple components (consumer).
// Poll is better when there are multiple consumers (e.g., multiple log tailers)
// reading from the same file.
type Poller interface {
	// Returns the poller ID.
	ID() string

	// Config returns the config used to start the poller.
	// This is useful for debugging and logging.
	Config() query_config.Config

	// Starts the poller routine.
	// Redundant calls will be skipped if there's an existing poller.
	Start(ctx context.Context, cfg query_config.Config, componentName string)
	// Stops the poller routine.
	// Safe to call multiple times.
	// Returns "true" if the poller was stopped with its reference count being zero.
	Stop(componentName string) bool

	// Last returns the last result.
	// Useful for constructing the state.
	Last() (*Item, error)
	// All returns all results.
	// Useful for constructing the events.
	All(since time.Time) ([]Item, error)
}

// Item is the basic unit of data that poller returns.
// If enabled, each result is persisted in the storage.
type Item struct {
	Time metav1.Time `json:"time"`

	// Generic component output.
	// Either Output or OutputEncoded should be set.
	Output any `json:"output,omitempty"`

	Error error `json:"error,omitempty"`
}

// Queries the component data from the host.
// Each get output is persisted to the storage if enabled.
type GetFunc func(context.Context) (any, error)

func New(id string, cfg query_config.Config, getFunc GetFunc) Poller {
	return &poller{
		id:                 id,
		tableName:          GetTableName(id),
		startPollFunc:      startPoll,
		getFunc:            getFunc,
		cfg:                cfg,
		inflightComponents: make(map[string]any),
	}
}

var _ Poller = (*poller)(nil)

type poller struct {
	id        string
	tableName string

	startPollFunc startPollFunc
	getFunc       GetFunc

	ctxMu  sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc

	cfgMu sync.RWMutex
	cfg   query_config.Config

	lastItemsMu sync.RWMutex
	lastItems   []Item

	inflightComponents map[string]any
}

type startPollFunc func(ctx context.Context, id string, interval time.Duration, get GetFunc) <-chan Item

func startPoll(ctx context.Context, id string, interval time.Duration, get GetFunc) <-chan Item {
	ch := make(chan Item, 1)
	go pollLoops(ctx, id, ch, interval, get)
	return ch
}

func pollLoops(ctx context.Context, id string, ch chan<- Item, interval time.Duration, get GetFunc) {
	// to get output very first time and start wait
	ticker := time.NewTicker(1)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			select {
			case ch <- Item{
				Time:  metav1.Time{Time: time.Now().UTC()},
				Error: ctx.Err(),
			}:
			default:
				log.Logger.Debugw("channel is full, skip this result and continue")
			}
			return

		case <-ticker.C:
			ticker.Reset(interval)
		}

		log.Logger.Debugw("polling", "id", id)

		output, err := get(ctx)
		if err != nil {
			log.Logger.Debugw("polling error", "id", id, "error", err)
			select {
			case <-ctx.Done():
				return
			case ch <- Item{
				Time:  metav1.Time{Time: time.Now().UTC()},
				Error: err,
			}:
			default:
				log.Logger.Debugw("channel is full, skip this result and continue")
			}
			continue
		}

		// maybe no state at the time
		if output == nil {
			continue
		}

		select {
		case <-ctx.Done():
			return
		case ch <- Item{
			Time:   metav1.Time{Time: time.Now().UTC()},
			Output: output,
		}:
		default:
			log.Logger.Debugw("channel is full, skip this result and continue")
		}
	}
}

func (pl *poller) ID() string {
	return pl.id
}

func (pl *poller) Config() query_config.Config {
	pl.cfgMu.RLock()
	defer pl.cfgMu.RUnlock()
	return pl.cfg
}

func GetTableName(componentName string) string {
	return "poll_results_" + state.ConvertToTableName(componentName)
}

// "caller" is used for reference counting
func (pl *poller) Start(ctx context.Context, cfg query_config.Config, componentName string) {
	log.Logger.Debugw("starting poller", "interval", cfg.Interval, "queueSize", cfg.QueueSize, "componentName", componentName)

	pl.ctxMu.Lock()
	defer pl.ctxMu.Unlock()

	pl.cfg = cfg

	pl.inflightComponents[componentName] = struct{}{}
	started := pl.ctx != nil
	if started {
		return
	}

	pl.ctx, pl.cancel = context.WithCancel(ctx)
	ch := pl.startPollFunc(pl.ctx, pl.id, cfg.Interval.Duration, pl.getFunc)
	go func() {
		for item := range ch {
			pl.processItem(item)
		}
	}()

	log.Logger.Debugw("started poller", "caller", componentName, "inflightComponents", len(pl.inflightComponents))
}

func (pl *poller) Stop(componentName string) bool {
	pl.ctxMu.Lock()
	defer pl.ctxMu.Unlock()

	log.Logger.Debugw("stopping the underlying poller", "componentName", componentName)

	stopped := pl.ctx == nil
	if stopped {
		log.Logger.Warnw("poller already stopped")
		return false
	}

	if len(pl.inflightComponents) == 0 {
		panic("inflightComponents is 0 but poller context is set -- should never happen")
	}
	delete(pl.inflightComponents, componentName)

	// do not cancel if there's any inflight component "after" this
	if len(pl.inflightComponents) > 0 {
		log.Logger.Debugw("skipping stopping the underlying poller -- inflights >0", "inflightComponents", len(pl.inflightComponents))
		return false
	}

	// noe, len(q.inflightComponents) == 0
	pl.cancel()
	pl.ctx = nil
	pl.cancel = nil
	log.Logger.Debugw("stopped poller", "caller", componentName)
	return true
}

func (pl *poller) processItem(item Item) {
	pl.ctxMu.RLock()
	canceled := pl.ctx == nil
	pl.ctxMu.RUnlock()

	if canceled {
		log.Logger.Warnw("poller already stopped -- skipping item")
		return
	}

	queueN := pl.Config().QueueSize

	pl.lastItemsMu.Lock()
	defer pl.lastItemsMu.Unlock()

	if queueN > 0 && len(pl.lastItems) >= queueN {
		pl.lastItems = pl.lastItems[1:]
	}
	pl.lastItems = append(pl.lastItems, item)
}

func (pl *poller) Last() (*Item, error) {
	pl.lastItemsMu.RLock()
	defer pl.lastItemsMu.RUnlock()

	if len(pl.lastItems) == 0 {
		return nil, nil
	}

	return &pl.lastItems[len(pl.lastItems)-1], nil
}

func (pl *poller) All(since time.Time) ([]Item, error) {
	pl.lastItemsMu.RLock()
	defer pl.lastItemsMu.RUnlock()

	// nothing in memory (e.g., process restart)
	// we removed db support to simplify the code
	if len(pl.lastItems) == 0 {
		return nil, nil
	}

	items := make([]Item, 0)
	for _, item := range pl.lastItems {
		if !since.IsZero() && item.Time.Time.Before(since) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}
