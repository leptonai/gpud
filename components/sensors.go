package components

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
	xidsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/xid"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"

	"github.com/leptonai/gpud/pkg/log"
)

// KernelObserver subscribes a component shim to health-validation observations.
type KernelObserver func(context.Context, func(context.Context, *observation.ObservationBatch)) (func(), error)

const sensorPollInterval = 100 * time.Millisecond

// MeasureSensor establishes GPUd's collection cycle. Data conversion is left to
// the component that requested the observations.
func MeasureSensor(ctx context.Context, sensor collect.Sensor, signals ...string) (*observation.ObservationBatch, error) {
	now := time.Now()
	cycle, err := collect.NewCycle("gpud."+sensor.ID()+"."+now.Format(time.RFC3339Nano), now)
	if err != nil {
		return nil, err
	}
	batch, err := sensor.Measure(ctx, collect.WithCycle(cycle), collect.WithSignalID(signals...))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if batch == nil {
		return nil, fmt.Errorf("sensor %s returned no observations", sensor.ID())
	}
	return batch, nil
}

// ObserveKernel shares the health-validation sensor among components of this
// GPUd instance. Each callback receives fresh observations in encounter order.
// The callback owns conversion and policy; a slow callback cannot block peers.
// Stop cancels delivery and waits for the callback before its event bucket closes.
func (g *GPUdInstance) ObserveKernel(ctx context.Context, consume func(context.Context, *observation.ObservationBatch)) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path := strings.TrimSpace(os.Getenv("KMSG_FILE_PATH")); path != "" && path != "/dev/kmsg" {
		return nil, fmt.Errorf("health-validation reads /dev/kmsg; KMSG_FILE_PATH=%q is unsupported", path)
	}
	g.kernelMu.Lock()
	if g.kernel == nil {
		source, err := kmsgsource.NewStream(0)
		if err != nil {
			g.kernelMu.Unlock()
			return nil, err
		}
		// The library's window only advances when immutable kernel identities
		// change. Avoid matching unchanged records again between component polls.
		var last recordID
		sensor, err := xidsensor.NewKmsg(func(ctx context.Context) ([]kmsgsource.Message, error) {
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
			g.kernelMu.Unlock()
			return nil, err
		}
		g.kernel = &sensorCycle{sensor: sensor, close: source.Close}
	}
	shared := g.kernel
	g.kernelUsers++
	g.kernelMu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		defer func() {
			g.kernelMu.Lock()
			defer g.kernelMu.Unlock()
			g.kernelUsers--
			if g.kernelUsers == 0 {
				if err := shared.close(); err != nil {
					log.Logger.Errorw("failed to close kernel source", "error", err)
				}
				g.kernel = nil
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
			batch, err := shared.measure(g.RootCtx)
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
		signals := make([]string, 0, len(s.sensor.Signals()))
		for _, signal := range s.sensor.Signals() {
			signals = append(signals, signal.ID())
		}
		batch, err := MeasureSensor(ctx, s.sensor, signals...)
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
					// A malformed Xid belongs to that record's consumer.
					// Untargeted failures mean the shared reader has failed.
					if o.GetTarget().GetKernelRecord() == nil && s.err == nil {
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
