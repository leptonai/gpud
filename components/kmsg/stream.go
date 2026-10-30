// Package kmsg shares health-validation kernel observations among GPUd components.
package kmsg

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"

	"github.com/leptonai/gpud/pkg/log"
)

// Stream owns one shared sensor and its subscriber lifetimes.
// The native reader opens on the first subscription and closes after the last.
type Stream struct {
	ctx   context.Context
	mu    sync.Mutex
	cycle *sensorCycle
	users int
}

// New creates a stream owned by the daemon's root context.
func New(ctx context.Context) *Stream { return &Stream{ctx: ctx} }

// Observer subscribes a component shim to health-validation observations.
type Observer func(context.Context, func(context.Context, *observation.ObservationBatch)) (func(), error)

const sensorPollInterval = 100 * time.Millisecond

// Subscribe delivers fresh observations in encounter order to a component.
// The callback owns conversion and policy; a slow callback cannot block peers.
// Stop cancels delivery and waits for the callback before its event bucket closes.
func (s *Stream) Subscribe(ctx context.Context, consume func(context.Context, *observation.ObservationBatch)) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path := strings.TrimSpace(os.Getenv("KMSG_FILE_PATH")); path != "" && path != "/dev/kmsg" {
		return nil, fmt.Errorf("health-validation reads /dev/kmsg; KMSG_FILE_PATH=%q is unsupported", path)
	}
	s.mu.Lock()
	if s.cycle == nil {
		source, err := kmsgsource.NewStream(0)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		// The library's window only advances when immutable kernel identities
		// change. Avoid matching unchanged records again between component polls.
		var last recordID
		sensor, err := kmsgsensor.New(func(ctx context.Context) ([]kmsgsource.Message, error) {
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
		}, map[string]string{"gpud.kernel_record": `(?s).*`})
		if err != nil {
			_ = source.Close()
			s.mu.Unlock()
			return nil, err
		}
		s.cycle = &sensorCycle{sensor: sensor, close: source.Close}
	}
	shared := s.cycle
	s.users++
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		defer func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.users--
			if s.users == 0 {
				if err := shared.close(); err != nil {
					log.Logger.Errorw("failed to close kernel source", "error", err)
				}
				s.cycle = nil
			}
		}()
		timer := time.NewTimer(0)
		defer timer.Stop()
		seen := make(map[string]int64)
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			batch, err := shared.measure(s.ctx)
			if batch != nil {
				fresh := freshKernelObservations(batch, seen)
				if len(fresh.Observations) > 0 {
					consume(ctx, fresh)
				}
			}
			if err != nil {
				if ctx.Err() == nil {
					log.Logger.Errorw("kernel sensor collection failed", "error", err)
				}
				return
			}
			timer.Reset(sensorPollInterval)
		}
	}()
	return func() { cancel(); <-done }, nil
}

// sensorCycle serializes a shared sensor's Measure calls and retains the batch
// for one polling interval. It holds no GPUd data types or event policy.
type sensorCycle struct {
	mu              sync.Mutex
	sensor          collect.Sensor
	close           func() error
	next            time.Time
	batch           *observation.ObservationBatch
	err             error
	overrunReported bool
}

func (s *sensorCycle) measure(ctx context.Context) (*observation.ObservationBatch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil && !time.Now().Before(s.next) {
		batch, err := Measure(ctx, s.sensor)
		s.err = err
		if batch != nil {
			delivered := &observation.ObservationBatch{CollectionId: batch.CollectionId}
			for _, o := range batch.Observations {
				if failure := o.GetCollectionError(); failure != nil {
					if recoverableKernelOverrun(failure) {
						if !s.overrunReported {
							log.Logger.Errorw("kernel records lost after source overrun; continuing collection", "error", failure.Detail)
							s.overrunReported = true
						}
						continue
					}
					if s.err == nil {
						s.err = fmt.Errorf("%s: %s", failure.Category, failure.Detail)
					}
				}
				delivered.Observations = append(delivered.Observations, o)
			}
			// An unchanged source window produces no observations. Preserve
			// the previous batch for late subscribers, including after EPIPE.
			if len(delivered.Observations) > 0 || s.batch == nil {
				s.batch = delivered
			}
		}

		s.next = time.Now().Add(sensorPollInterval)
	}
	return s.batch, s.err
}

func freshKernelObservations(batch *observation.ObservationBatch, seen map[string]int64) *observation.ObservationBatch {
	fresh := &observation.ObservationBatch{CollectionId: batch.CollectionId}
	next := make(map[string]int64)
	for _, o := range batch.Observations {
		target := o.GetTarget().GetKernelRecord()
		if target == nil || target.GetBootId() == "" || target.GetSequence() < 0 {
			fresh.Observations = append(fresh.Observations, o)
			continue
		}
		boot, seq := target.GetBootId(), target.GetSequence()
		if last, ok := seen[boot]; ok && seq <= last {
			continue
		}
		fresh.Observations = append(fresh.Observations, o)
		previous, ok := next[boot]
		if !ok {
			previous, ok = seen[boot]
		}
		if ok && seq > previous+1 {
			log.Logger.Errorw("kernel records lost before component delivery", "bootID", boot, "after", previous, "before", seq)
		}
		if !ok || seq > previous {
			next[boot] = seq
		}
	}
	for boot, seq := range next {
		seen[boot] = seq
	}
	return fresh
}

// Only the library's typed overrun classification is recoverable. A joined
// EPIPE and terminal error does not carry this code (health-validation #247).
func recoverableKernelOverrun(failure *observation.CollectionError) bool {
	return failure.Category == observation.CollectionErrorCategorySourceError && failure.SourceError == kmsgsensor.SourceErrorOverrun
}
