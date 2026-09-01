package nvsentinel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvsentinel/proto/datamodels"
)

const (
	// DefaultSocketPath is the default unix socket GPUd serves. The
	// NVSentinel DaemonSet mounts host /var/run/nvsentinel at container
	// /var/run, so the platform-connector reaches this socket at container
	// path /var/run/gpud.sock.
	DefaultSocketPath = "/var/run/nvsentinel/gpud.sock"

	// DefaultEventDedupWindow is how long a received NVSentinel event
	// suppresses GPUd's own duplicate detection of the same data point.
	DefaultEventDedupWindow = 2 * time.Minute

	// maxRecentEvents caps the in-memory recent-event index. The index
	// answers Covers queries for duplicate suppression; entries are added
	// by RecordCoverage only after a component persisted the data point.
	// It stays small because Covers queries always use a short time window.
	maxRecentEvents = 4096

	// receiverQueueSize bounds the internal receive queue. The gRPC
	// handler acknowledges a batch only after every event is enqueued
	// here, so this queue is the acknowledgment boundary: once enqueued,
	// the dispatcher keeps the event until every registered subscriber has
	// accepted it. When the queue is full the RPC fails with Unavailable
	// and the NVSentinel gRPC sink retries instead of marking the queue
	// item complete, so a stuck or slow consumer cannot silently lose an
	// event.
	receiverQueueSize = 4096

	// subscriberBufferSize bounds each subscriber channel. Subscribers are
	// component-internal forwarders that drain events immediately. A full
	// channel means a subscriber is stuck; the dispatcher then waits for
	// that subscriber instead of dropping the event, pushing backpressure
	// onto the receive queue and — once that fills — onto NVSentinel's
	// retry path.
	subscriberBufferSize = 256
)

// errSourceClosed is returned to the gRPC handler when an event arrives
// after the source closed. The platform connector sees a failed RPC and
// keeps the queue item for a retry.
var errSourceClosed = errors.New("nvsentinel source is closed")

// errQueueFull is returned to the gRPC handler when the receive queue is
// full. The RPC fails with codes.Unavailable so the NVSentinel gRPC sink
// retries the batch rather than acknowledging undelivered events.
var errQueueFull = errors.New("nvsentinel receive queue is full")

// Source is the GPUd-side view of a local NVSentinel deployment. It receives
// health events forwarded by the node's NVSentinel platform-connector and
// answers "has NVSentinel reported this data point recently" queries.
type Source interface {
	// Subscribe registers a listener for every received health event.
	// It returns the event channel and an unsubscribe function. The
	// unsubscribe function detaches the listener and is safe to call
	// twice; the channel itself is closed when the source is closed.
	// Events received before the first subscriber registers are held in
	// the receive queue and delivered once a subscriber appears, so
	// events are not lost while components are still starting.
	Subscribe() (<-chan HealthEvent, func())

	// RecordCoverage marks a received event as durably handled by a
	// component. Components call it only after the event's data point is
	// persisted (or an identical data point is already stored). Covers
	// matches only events reported through RecordCoverage, so coverage
	// always represents successful component persistence: a dropped
	// broadcast or a failed event-store insert never suppresses GPUd's
	// native detection of the same incident. Healthy events carry no new
	// data point and are ignored.
	RecordCoverage(ev HealthEvent)

	// Covers reports whether a persisted event satisfied the predicate
	// within the given window. Components call it to decide whether to
	// suppress their own duplicate detection.
	Covers(window time.Duration, match func(HealthEvent) bool) bool

	// LastReceived returns when the last event batch was received.
	// It returns the zero time if no event has been received.
	LastReceived() time.Time

	// Close stops the receiver, closes all subscriber channels, and removes
	// the socket file.
	Close() error
}

// New starts a receiver that serves the NVSentinel PlatformConnector
// gRPC API on the given unix socket path. The NVSentinel platform-connector
// gRPC sink target must point at this socket (helm values
// platformConnector.grpcSinkConnector.enabled/target).
func New(socketPath string) (Source, error) {
	if socketPath == "" {
		return nil, errors.New("nvsentinel socket path is required")
	}
	if !filepath.IsAbs(socketPath) {
		return nil, fmt.Errorf("nvsentinel socket path must be absolute, got %q", socketPath)
	}

	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create nvsentinel socket directory: %w", err)
	}

	// A stale socket file can survive a crash; remove it before listening.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to remove stale nvsentinel socket: %w", err)
	}

	lc := &net.ListenConfig{}
	lis, err := lc.Listen(context.Background(), "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on nvsentinel socket %s: %w", socketPath, err)
	}

	// NVSentinel platform-connectors run as root. GPUd runs as root too.
	if err := os.Chmod(socketPath, 0o660); err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("failed to set nvsentinel socket permissions: %w", err)
	}

	s := &source{
		socketPath:   socketPath,
		lis:          lis,
		srv:          grpc.NewServer(),
		queue:        make(chan HealthEvent, receiverQueueSize),
		closing:      make(chan struct{}),
		dispatchDone: make(chan struct{}),
		subs:         make(map[int]*subscription),
		subAdded:     make(chan struct{}),
	}
	datamodels.RegisterPlatformConnectorServer(s.srv, &platformConnectorServer{src: s})

	go s.dispatchLoop()
	go func() {
		if err := s.srv.Serve(lis); err != nil {
			log.Logger.Errorw("nvsentinel receiver stopped serving", "socket", socketPath, "error", err)
		}
	}()

	log.Logger.Infow("nvsentinel receiver listening", "socket", socketPath)
	return s, nil
}

var _ Source = &source{}

// subscription is one registered listener. The dispatcher sends on ch;
// done is closed by unsubscribe so a dispatcher blocked on a full channel
// stops waiting on a consumer that has gone away. ch itself is closed
// only after the dispatcher has exited (source Close), so a send can
// never race a close.
type subscription struct {
	ch   chan HealthEvent
	done chan struct{}
}

type source struct {
	socketPath string
	lis        net.Listener
	srv        *grpc.Server

	// queue is the acknowledged receive queue: the gRPC handler returns
	// success only after the event lands here, and the dispatcher drains
	// it in order, delivering every event to every registered subscriber.
	queue chan HealthEvent
	// closing is closed by Close to unblock a dispatcher that is waiting
	// on a full subscriber channel or on the first subscriber.
	closing chan struct{}
	// dispatchDone is closed when the dispatcher goroutine exits. Close
	// waits for it before closing subscriber channels.
	dispatchDone chan struct{}

	mu     sync.Mutex
	recent []HealthEvent // oldest first
	subs   map[int]*subscription
	// subAdded is closed and replaced whenever a subscriber registers, so
	// a dispatcher holding an event for zero subscribers wakes up.
	subAdded     chan struct{}
	nextSubID    int
	lastReceived time.Time
	closed       bool
}

func (s *source) Subscribe() (<-chan HealthEvent, func()) {
	sub := &subscription{
		ch:   make(chan HealthEvent, subscriberBufferSize),
		done: make(chan struct{}),
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		close(sub.ch)
		return sub.ch, func() {}
	}
	id := s.nextSubID
	s.nextSubID++
	s.subs[id] = sub
	// Wake a dispatcher that is holding an event for zero subscribers.
	close(s.subAdded)
	s.subAdded = make(chan struct{})
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(sub.done)
		}
	}
	return sub.ch, unsubscribe
}

func (s *source) Covers(window time.Duration, match func(HealthEvent) bool) bool {
	cutoff := time.Now().UTC().Add(-window)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Do not prune by the caller's window. A short-window query must not
	// discard events a later wider query can still see. The count cap in
	// record already bounds memory.
	for i := len(s.recent) - 1; i >= 0; i-- {
		ev := s.recent[i]
		if ev.GeneratedTimestamp.After(cutoff) && match(ev) {
			return true
		}
	}
	return false
}

func (s *source) LastReceived() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastReceived
}

func (s *source) Close() error {
	// grpc Server.Stop already closes the listener; tolerate the double-close.
	s.srv.Stop()
	err := s.lis.Close()
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		_ = os.Remove(s.socketPath)
		return nil
	}
	s.closed = true
	close(s.closing)
	s.mu.Unlock()

	// Wait for the dispatcher to exit before closing subscriber channels:
	// it is the only sender, so afterwards a close cannot race a send.
	<-s.dispatchDone

	s.mu.Lock()
	for id, sub := range s.subs {
		close(sub.ch)
		delete(s.subs, id)
	}
	s.mu.Unlock()

	_ = os.Remove(s.socketPath)
	return nil
}

// enqueue adds an event to the acknowledged receive queue. The gRPC
// handler returns an error — and NVSentinel retries — when the event
// cannot be enqueued, so a nil RPC response always means the event is
// durably inside GPUd's delivery pipeline.
func (s *source) enqueue(ev HealthEvent) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errSourceClosed
	}
	s.lastReceived = time.Now().UTC()
	s.mu.Unlock()

	select {
	case s.queue <- ev:
		return nil
	default:
		return errQueueFull
	}
}

// dispatchLoop drains the receive queue in order and delivers every event
// to every registered subscriber. It exits when the source is closing.
func (s *source) dispatchLoop() {
	defer close(s.dispatchDone)
	for {
		select {
		case <-s.closing:
			return
		case ev := <-s.queue:
			if !s.deliver(ev) {
				return
			}
		}
	}
}

// deliver broadcasts one event to every registered subscriber and reports
// whether delivery completed. Sends block until the subscriber accepts the
// event: a stuck subscriber stalls the queue and ultimately the RPC
// (NVSentinel then retries) rather than silently losing the event. With
// zero subscribers the event is held until one registers — events received
// while components are still starting are not dropped. It returns false
// when the source is closing mid-delivery.
func (s *source) deliver(ev HealthEvent) bool {
	for {
		s.mu.Lock()
		if len(s.subs) == 0 {
			subAdded := s.subAdded
			s.mu.Unlock()
			select {
			case <-subAdded:
			case <-s.closing:
				return false
			}
			continue
		}
		subs := make([]*subscription, 0, len(s.subs))
		for _, sub := range s.subs {
			subs = append(subs, sub)
		}
		s.mu.Unlock()

		for _, sub := range subs {
			select {
			case sub.ch <- ev:
			case <-sub.done:
				// Unsubscribed mid-delivery; the consumer is gone.
			case <-s.closing:
				return false
			}
		}
		return true
	}
}

// RecordCoverage implements Source.RecordCoverage.
func (s *source) RecordCoverage(ev HealthEvent) {
	if ev.IsHealthy {
		// A healthy event carries no new data point. It must never
		// suppress native detection of a later unhealthy incident.
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.recent = append(s.recent, ev)
	if len(s.recent) > maxRecentEvents {
		s.recent = s.recent[len(s.recent)-maxRecentEvents:]
	}
}

var _ datamodels.PlatformConnectorServer = &platformConnectorServer{}

// platformConnectorServer implements the NVSentinel PlatformConnector API.
type platformConnectorServer struct {
	datamodels.UnimplementedPlatformConnectorServer
	src *source
}

func (s *platformConnectorServer) HealthEventOccurredV1(_ context.Context, events *datamodels.HealthEvents) (*emptypb.Empty, error) {
	receivedAt := time.Now().UTC()
	for _, pbEv := range events.GetEvents() {
		ev := healthEventFromProto(pbEv, receivedAt)
		log.Logger.Debugw("received nvsentinel health event",
			"agent", ev.Agent, "checkName", ev.CheckName,
			"componentClass", ev.ComponentClass, "isHealthy", ev.IsHealthy)
		if err := s.src.enqueue(ev); err != nil {
			// The event never entered the acknowledged receive queue, so
			// the RPC must fail: a nil response would make the NVSentinel
			// gRPC sink mark the queue item complete and skip retry,
			// permanently losing the event. codes.Unavailable is the
			// retryable signal. The connector retries the whole batch;
			// already-enqueued events are deduplicated downstream by the
			// components' twin checks.
			return nil, status.Error(codes.Unavailable,
				fmt.Sprintf("failed to accept nvsentinel health event %q: %v", ev.ID, err))
		}
	}
	return &emptypb.Empty{}, nil
}
