package kmsg

import (
	"context"
	"sync"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
)

type stream interface {
	Read(context.Context) ([]kmsgsource.Message, error)
	Close() error
}

var sharedWatchers = watcherPool{open: func() (stream, error) { return kmsgsource.NewStream(0) }}

// watcherPool owns the native reader for all live subscriptions, including ones
// whose Watch method has not started yet. Opening and final closing are serialized
// so component startup/shutdown cannot accidentally create competing readers.
type watcherPool struct {
	mu         sync.Mutex
	open       func() (stream, error)
	collection *sharedCollection
	refs       int
}

func (p *watcherPool) newWatcher(opts ...OpOption) (*watcher, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.collection == nil {
		source, err := p.open()
		if err != nil {
			return nil, err
		}
		// Kernel record identities are immutable and the stream window only
		// advances as new records arrive. An unchanged tail needs no matching or
		// conversion; subscribers can keep using the last converted window.
		var last recordID
		sensor, err := newSensor(func(ctx context.Context) ([]kmsgsource.Message, error) {
			records, err := source.Read(ctx)
			if err == nil && len(records) > 0 {
				tail := records[len(records)-1]
				id := recordID{bootID: tail.BootID, sequence: tail.Sequence}
				if id.bootID != "" && id == last {
					return nil, nil
				}
				last = id
			}
			return records, err
		})
		if err != nil {
			_ = source.Close()
			return nil, err
		}
		ctx, cancel := context.WithCancel(context.Background())
		p.collection = &sharedCollection{sensor: sensor, source: source, ctx: ctx, cancel: cancel}
	}
	p.refs++
	ctx, cancel := context.WithCancel(context.Background())
	return &watcher{
		collection: p.collection, release: p.release, ctx: ctx, cancel: cancel,
		deduperOpts: append([]OpOption(nil), opts...),
	}, nil
}

func (p *watcherPool) release() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refs--
	if p.refs != 0 {
		return nil
	}
	p.collection.cancel()
	err := p.collection.source.Close()
	p.collection = nil
	return err
}

// sharedCollection measures the library's non-destructive stream window at most
// once per polling interval, regardless of the number of components. The sensor
// rejects concurrent Measure calls, so serialize them here and share the converted
// immutable snapshot. Delivery and deduplication happen outside this lock: a slow
// subscriber can miss records as the window advances, but cannot block its peers.
type sharedCollection struct {
	sensor collect.Sensor
	source stream
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	nextRead time.Time
	records  []Message
	err      error
}

func (c *sharedCollection) read() ([]Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Keep a partial terminal result available to every subscriber, rather than
	// letting the first component consume or clear a collection failure.
	if c.err == nil && !time.Now().Before(c.nextRead) {
		records, err := collectMessages(c.ctx, c.sensor)
		if len(records) > 0 || err != nil {
			c.records = records
		}
		c.err = err
		c.nextRead = time.Now().Add(streamPollInterval)
	}
	return c.records, c.err
}
