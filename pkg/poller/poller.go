package poller

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/leptonai/gpud/log"
	poller_config "github.com/leptonai/gpud/pkg/poller/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var ErrNoData = errors.New("no data collected yet in the poller")

// Defines the common poller interface.
// It polls the data source (rather than watch) in order
// to share the same data source with multiple components (consumer).
// Poll is better when there are multiple consumers (e.g., multiple log tailers)
// reading from the same file.
type Poller interface {
	// Returns the poller ID.
	ID() string

	// Starts the poller routine.
	// Redundant calls will be skipped if there's an existing poller.
	Start(ctx context.Context, callerName string)

	// Config returns the config used to start the poller.
	// This is useful for debugging and logging.
	Config() poller_config.Config

	// Stops the poller routine.
	// Safe to call multiple times.
	// Returns "true" if the poller was stopped with its reference count being zero.
	Stop(callerName string) bool

	// Last returns the last item in the queue.
	// It returns ErrNoData if no item is collected yet.
	Last() (*Item, error)

	// LastSuccess returns the last item in the queue with no error.
	// It returns ErrNoData if no such item is collected yet.
	LastSuccess() (*Item, error)

	// Returns the last known error in the queue.
	// Returns "ErrNoData" if no data is found.
	// Returns nil if no error is found.
	LastError() error

	// All returns all results in the queue since the given time.
	// It returns ErrNoData if no item is collected yet.
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

// GetErrHandler is a function that handles the error from the get operation.
type GetErrHandler func(error) error

func New(id string, cfg poller_config.Config, getFunc GetFunc, getErrHandler GetErrHandler) Poller {
	if getErrHandler == nil {
		getErrHandler = func(err error) error {
			return err
		}
	}
	return &pollerImpl{
		id:                 id,
		startPollFunc:      startPoll,
		getFunc:            getFunc,
		getErrHandler:      getErrHandler,
		cfg:                cfg,
		inflightComponents: make(map[string]any),
	}
}

var _ Poller = (*pollerImpl)(nil)

type pollerImpl struct {
	id string

	startPollFunc startPollFunc
	getFunc       GetFunc
	getErrHandler GetErrHandler

	ctxMu  sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc

	cfgMu sync.RWMutex
	cfg   poller_config.Config

	lastItemsMu sync.RWMutex
	lastItems   []Item

	inflightComponents map[string]any
}

type startPollFunc func(ctx context.Context, id string, interval time.Duration, getTimeout time.Duration, getFunc GetFunc, getErrHandler GetErrHandler) <-chan Item

func startPoll(ctx context.Context, id string, interval time.Duration, getTimeout time.Duration, getFunc GetFunc, getErrHandler GetErrHandler) <-chan Item {
	ch := make(chan Item, 1)
	go pollLoops(ctx, id, ch, interval, getTimeout, getFunc, getErrHandler)
	return ch
}

func pollLoops(ctx context.Context, id string, ch chan<- Item, interval time.Duration, getTimeout time.Duration, getFunc GetFunc, getErrHandler GetErrHandler) {
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

		// to prevent component from being blocked by the get operation
		var cctx context.Context
		var ccancel context.CancelFunc
		if getTimeout > 0 {
			cctx, ccancel = context.WithTimeout(ctx, getTimeout)
		} else {
			cctx = ctx
			ccancel = func() {}
		}
		output, err := getFunc(cctx)
		ccancel()

		err = getErrHandler(err)

		// maybe no state at the time
		if output == nil && err == nil {
			continue
		}

		select {
		case <-ctx.Done():
			return

		case ch <- Item{
			Time:   metav1.Time{Time: time.Now().UTC()},
			Output: output,
			Error:  err,
		}:
			if err != nil {
				log.Logger.Debugw("polling error", "id", id, "error", err)
			}

		default:
			log.Logger.Debugw("channel is full, skip this result and continue")
		}
	}
}

func (pl *pollerImpl) ID() string {
	return pl.id
}

func (pl *pollerImpl) Config() poller_config.Config {
	pl.cfgMu.RLock()
	defer pl.cfgMu.RUnlock()
	return pl.cfg
}

// "caller" is used for reference counting
func (pl *pollerImpl) Start(ctx context.Context, callerName string) {
	log.Logger.Debugw("starting poller", "interval", pl.cfg.Interval, "queueSize", pl.cfg.QueueSize, "callerName", callerName)

	pl.ctxMu.Lock()
	defer pl.ctxMu.Unlock()

	pl.inflightComponents[callerName] = struct{}{}
	started := pl.ctx != nil
	if started {
		return
	}

	pl.ctx, pl.cancel = context.WithCancel(ctx)
	ch := pl.startPollFunc(pl.ctx, pl.id, pl.cfg.Interval.Duration, pl.cfg.GetTimeout.Duration, pl.getFunc, pl.getErrHandler)
	go func() {
		for item := range ch {
			pl.processItem(item)
		}
	}()

	log.Logger.Debugw("started poller", "caller", callerName, "inflightComponents", len(pl.inflightComponents))
}

func (pl *pollerImpl) Stop(callerName string) bool {
	pl.ctxMu.Lock()
	defer pl.ctxMu.Unlock()

	log.Logger.Debugw("stopping the underlying poller", "caller", callerName)

	stopped := pl.ctx == nil
	if stopped {
		log.Logger.Warnw("poller already stopped")
		return false
	}

	if len(pl.inflightComponents) == 0 {
		panic("inflightComponents is 0 but poller context is set -- should never happen")
	}
	delete(pl.inflightComponents, callerName)

	// do not cancel if there's any inflight component "after" this
	if len(pl.inflightComponents) > 0 {
		log.Logger.Debugw("skipping stopping the underlying poller -- inflights >0", "inflightComponents", len(pl.inflightComponents))
		return false
	}

	// noe, len(q.inflightComponents) == 0
	pl.cancel()
	pl.ctx = nil
	pl.cancel = nil
	log.Logger.Debugw("stopped poller", "caller", callerName)
	return true
}

func (pl *pollerImpl) processItem(item Item) {
	pl.ctxMu.RLock()
	canceled := pl.ctx == nil
	pl.ctxMu.RUnlock()

	if canceled {
		log.Logger.Warnw("poller already stopped -- skipping item")
		return
	}

	pl.insertItemToInMemoryQueue(item)
}

func (pl *pollerImpl) insertItemToInMemoryQueue(item Item) {
	queueN := pl.Config().QueueSize

	pl.lastItemsMu.Lock()
	defer pl.lastItemsMu.Unlock()

	if queueN > 0 && len(pl.lastItems) >= queueN {
		pl.lastItems = pl.lastItems[1:]
	}
	pl.lastItems = append(pl.lastItems, item)
}

// Last returns the last item in the queue.
// It returns ErrNoData if no item is collected yet.
func (pl *pollerImpl) Last() (*Item, error) {
	return pl.readLast(false)
}

// LastSuccess returns the last item in the queue with no error.
// It returns ErrNoData if no item is collected yet.
func (pl *pollerImpl) LastSuccess() (*Item, error) {
	return pl.readLast(true)
}

// Reads the last item from the queue.
// If requireNoErr is true, it returns the last item with no error.
// If no item is found, it returns ErrNoData.
func (pl *pollerImpl) readLast(requireNoErr bool) (*Item, error) {
	pl.lastItemsMu.RLock()
	defer pl.lastItemsMu.RUnlock()

	if len(pl.lastItems) == 0 {
		return nil, ErrNoData
	}

	// reverse iterate
	for i := len(pl.lastItems) - 1; i >= 0; i-- {
		item := pl.lastItems[i]
		if requireNoErr && item.Error != nil {
			log.Logger.Warnw("skipping item due to error", "id", pl.id, "error", item.Error)
			continue
		}
		return &item, nil
	}

	return nil, ErrNoData
}

// Returns the last known error in the queue.
// Returns "ErrNoData" if no data is found.
// Returns nil if no error is found.
func (pl *pollerImpl) LastError() error {
	return pl.readLastErr()
}

// Returns the last known error in the queue.
// Returns "ErrNoData" if no data is found.
// Returns nil if no error is found.
func (pl *pollerImpl) readLastErr() error {
	pl.lastItemsMu.RLock()
	defer pl.lastItemsMu.RUnlock()

	if len(pl.lastItems) == 0 {
		return ErrNoData
	}

	// reverse iterate
	for i := len(pl.lastItems) - 1; i >= 0; i-- {
		item := pl.lastItems[i]
		if item.Error != nil {
			return item.Error
		}
	}

	return nil
}

// All returns all results in the queue since the given time.
// It returns ErrNoData if no item is collected yet.
func (pl *pollerImpl) All(since time.Time) ([]Item, error) {
	return pl.readAllItemsFromInMemoryQueue(since)
}

func (pl *pollerImpl) readAllItemsFromInMemoryQueue(since time.Time) ([]Item, error) {
	pl.lastItemsMu.RLock()
	defer pl.lastItemsMu.RUnlock()

	// nothing in memory (e.g., process restart)
	if len(pl.lastItems) == 0 {
		return nil, ErrNoData
	}

	items := make([]Item, 0)
	for _, item := range pl.lastItems {
		if !since.IsZero() && item.Time.Time.Before(since) {
			continue
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		return nil, ErrNoData
	}

	return items, nil
}
