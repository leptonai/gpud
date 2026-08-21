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
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/nvsentinel/proto/datamodels"
)

const (
	// DefaultSocketPath is the default unix socket GPUd serves for NVSentinel
	// health event forwarding. The stock NVSentinel DaemonSet mounts the
	// host's /var/run/nvsentinel directory at container path /var/run, so the
	// platform-connector reaches this socket at container path
	// /var/run/gpud.sock.
	DefaultSocketPath = "/var/run/nvsentinel/gpud.sock"

	// DefaultEventDedupWindow is how long a received NVSentinel event
	// suppresses GPUd's own duplicate detection of the same data point.
	DefaultEventDedupWindow = 2 * time.Minute

	// maxRecentEvents caps the in-memory recent-event index. The index exists
	// to answer Covers queries for duplicate suppression; it does not need to
	// be large because Covers queries always use a short time window.
	maxRecentEvents = 4096

	// subscriberBufferSize bounds each subscriber channel. Subscribers are
	// component-internal forwarders that drain immediately, so a full channel
	// means a subscriber is stuck; the event is dropped and logged.
	subscriberBufferSize = 256
)

// Source is the GPUd-side view of a local NVSentinel deployment. It receives
// health events forwarded by the node's NVSentinel platform-connector and
// answers "has NVSentinel reported this data point recently" queries.
type Source interface {
	// Subscribe registers a listener for every received health event.
	// It returns the event channel and an unsubscribe function.
	// The channel is closed when the source is closed.
	Subscribe() (<-chan HealthEvent, func())

	// Covers reports whether a received event matches match within the given
	// window. Components use it to prefer NVSentinel data points over their
	// own duplicate detection.
	Covers(window time.Duration, match func(HealthEvent) bool) bool

	// LastReceived returns when the last event batch was received.
	// It returns the zero time if no event has been received.
	LastReceived() time.Time

	// Close stops the receiver and closes all subscriber channels.
	Close() error
}

// New starts a receiver that serves the NVSentinel PlatformConnector gRPC API
// on the given unix socket path. Point the NVSentinel platform-connector gRPC
// sink target at this socket (helm values
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

	// NVSentinel platform-connectors run as root, same as GPUd.
	if err := os.Chmod(socketPath, 0o660); err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("failed to set nvsentinel socket permissions: %w", err)
	}

	s := &source{
		socketPath: socketPath,
		lis:        lis,
		srv:        grpc.NewServer(),
		subs:       make(map[int]chan HealthEvent),
	}
	datamodels.RegisterPlatformConnectorServer(s.srv, &platformConnectorServer{src: s})

	go func() {
		if err := s.srv.Serve(lis); err != nil {
			log.Logger.Errorw("nvsentinel receiver stopped serving", "socket", socketPath, "error", err)
		}
	}()

	log.Logger.Infow("nvsentinel receiver listening", "socket", socketPath)
	return s, nil
}

var _ Source = &source{}

type source struct {
	socketPath string
	lis        net.Listener
	srv        *grpc.Server

	mu           sync.Mutex
	recent       []HealthEvent // oldest first
	subs         map[int]chan HealthEvent
	nextSubID    int
	lastReceived time.Time
	closed       bool
}

func (s *source) Subscribe() (<-chan HealthEvent, func()) {
	ch := make(chan HealthEvent, subscriberBufferSize)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	id := s.nextSubID
	s.nextSubID++
	s.subs[id] = ch
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if sub, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(sub)
		}
	}
	return ch, unsubscribe
}

func (s *source) Covers(window time.Duration, match func(HealthEvent) bool) bool {
	cutoff := time.Now().UTC().Add(-window)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Do not prune by the caller's window here: a short-window query must not
	// discard events that a later, wider query can still legitimately see.
	// The fixed cap in record already bounds memory.
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
	if !s.closed {
		s.closed = true
		for id, ch := range s.subs {
			close(ch)
			delete(s.subs, id)
		}
	}
	s.mu.Unlock()

	_ = os.Remove(s.socketPath)
	return nil
}

// record stores one received event and forwards it to all subscribers.
func (s *source) record(ev HealthEvent) {
	s.mu.Lock()
	s.lastReceived = time.Now().UTC()
	s.recent = append(s.recent, ev)
	if len(s.recent) > maxRecentEvents {
		s.recent = s.recent[len(s.recent)-maxRecentEvents:]
	}
	subs := make([]chan HealthEvent, 0, len(s.subs))
	for _, ch := range s.subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			log.Logger.Errorw("nvsentinel subscriber channel full, dropping event",
				"checkName", ev.CheckName, "componentClass", ev.ComponentClass)
		}
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
		s.src.record(ev)
	}
	return &emptypb.Empty{}, nil
}
