package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsnvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/process"
)

type Op struct {
	auditLogger         log.AuditLogger
	machineID           string
	pipeInterval        time.Duration
	enableAutoUpdate    bool
	autoUpdateExitCode  int
	componentsRegistry  components.Registry
	nvmlInstance        nvidianvml.Instance
	metricsStore        pkgmetrics.Store
	savePluginSpecsFunc func(context.Context, pkgcustomplugins.Specs) (bool, error)
	faultInjector       pkgfaultinjector.Injector
}

type OpOption func(*Op)

var ErrAutoUpdateDisabledButExitCodeSet = errors.New("auto update is disabled but auto update by exit code is set")

func (op *Op) applyOpts(opts []OpOption) error {
	op.autoUpdateExitCode = -1

	for _, opt := range opts {
		opt(op)
	}

	if op.auditLogger == nil {
		op.auditLogger = log.NewNopAuditLogger()
	}

	if !op.enableAutoUpdate && op.autoUpdateExitCode != -1 {
		return ErrAutoUpdateDisabledButExitCodeSet
	}

	return nil
}

func WithAuditLogger(auditLogger log.AuditLogger) OpOption {
	return func(op *Op) {
		op.auditLogger = auditLogger
	}
}

func WithMachineID(machineID string) OpOption {
	return func(op *Op) {
		op.machineID = machineID
	}
}

func WithPipeInterval(t time.Duration) OpOption {
	return func(op *Op) {
		op.pipeInterval = t
	}
}

func WithEnableAutoUpdate(enableAutoUpdate bool) OpOption {
	return func(op *Op) {
		op.enableAutoUpdate = enableAutoUpdate
	}
}

func WithComponentsRegistry(componentsRegistry components.Registry) OpOption {
	return func(op *Op) {
		op.componentsRegistry = componentsRegistry
	}
}

func WithNvidiaInstance(nvmlInstance nvidianvml.Instance) OpOption {
	return func(op *Op) {
		op.nvmlInstance = nvmlInstance
	}
}

func WithMetricsStore(metricsStore pkgmetrics.Store) OpOption {
	return func(op *Op) {
		op.metricsStore = metricsStore
	}
}

func WithSavePluginSpecsFunc(savePluginSpecsFunc func(context.Context, pkgcustomplugins.Specs) (bool, error)) OpOption {
	return func(op *Op) {
		op.savePluginSpecsFunc = savePluginSpecsFunc
	}
}

func WithFaultInjector(faultInjector pkgfaultinjector.Injector) OpOption {
	return func(op *Op) {
		op.faultInjector = faultInjector
	}
}

// Triggers an auto update of GPUd itself by exiting the process with the given exit code.
// Useful when the machine is managed by the Kubernetes daemonset and we want to
// trigger an auto update when the daemonset restarts the machine.
func WithAutoUpdateExitCode(autoUpdateExitCode int) OpOption {
	return func(op *Op) {
		op.autoUpdateExitCode = autoUpdateExitCode
	}
}

type Session struct {
	ctx    context.Context
	cancel context.CancelFunc

	pipeInterval time.Duration

	auditLogger log.AuditLogger

	machineID string

	// epLocalGPUdServer is the endpoint of the local GPUd server
	epLocalGPUdServer string
	// epControlPlane is the endpoint of the control plane
	epControlPlane string

	token string

	createGossipRequestFunc func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error)

	setDefaultIbExpectedPortStatesFunc func(states infiniband.ExpectedPortStates)
	setDefaultGPUCountsFunc            func(counts componentsnvidiagpucounts.ExpectedGPUCounts)
	setDefaultNFSGroupConfigsFunc      func(cfgs pkgnfschecker.Configs)
	setDefaultXIDRebootThresholdFunc   func(threshold componentsxid.RebootThreshold)

	nvmlInstance       nvidianvml.Instance
	metricsStore       pkgmetrics.Store
	componentsRegistry components.Registry
	processRunner      process.Runner

	components []string

	closer *closeOnce

	writer chan Body
	reader chan Body

	enableAutoUpdate   bool
	autoUpdateExitCode int

	savePluginSpecsFunc func(context.Context, pkgcustomplugins.Specs) (bool, error)
	faultInjector       pkgfaultinjector.Injector

	lastPackageTimestampMu sync.RWMutex
	lastPackageTimestamp   time.Time

	// Testable functions for dependency injection
	// These allow unit tests to mock time operations, sleep, and connection creation

	// timeAfterFunc returns a channel that receives after duration d
	// In production: time.After, in tests: can be mocked for instant return
	timeAfterFunc func(d time.Duration) <-chan time.Time

	// timeSleepFunc sleeps for duration d
	// In production: time.Sleep, in tests: can be mocked to skip sleep
	timeSleepFunc func(d time.Duration)

	// startReaderFunc starts a reader connection
	// In production: s.startReader, in tests: can be mocked
	startReaderFunc func(ctx context.Context, readerExit chan any, jar *cookiejar.Jar)

	// startWriterFunc starts a writer connection
	// In production: s.startWriter, in tests: can be mocked
	startWriterFunc func(ctx context.Context, writerExit chan any, jar *cookiejar.Jar)

	// checkServerHealthFunc checks server health
	// In production: s.checkServerHealth, in tests: can be mocked
	checkServerHealthFunc func(ctx context.Context, jar *cookiejar.Jar) error
}

type closeOnce struct {
	closer chan any
	once   sync.Once
	sync.RWMutex
}

func (c *closeOnce) Close() {
	c.once.Do(func() {
		close(c.closer)
	})
}

func (c *closeOnce) Done() chan any {
	return c.closer
}

func NewSession(ctx context.Context, epLocalGPUdServer string, epControlPlane string, token string, opts ...OpOption) (*Session, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	cps := make([]string, 0)
	for _, c := range op.componentsRegistry.All() {
		cps = append(cps, c.Name())
	}

	cctx, ccancel := context.WithCancel(ctx)
	s := &Session{
		ctx:    cctx,
		cancel: ccancel,

		pipeInterval: op.pipeInterval,

		auditLogger: op.auditLogger,

		epLocalGPUdServer: epLocalGPUdServer,
		epControlPlane:    epControlPlane,

		machineID: op.machineID,
		token:     token,

		createGossipRequestFunc: pkgmachineinfo.CreateGossipRequest,

		setDefaultIbExpectedPortStatesFunc: componentsnvidiainfiniband.SetDefaultExpectedPortStates,
		setDefaultGPUCountsFunc:            componentsnvidiagpucounts.SetDefaultExpectedGPUCounts,
		setDefaultNFSGroupConfigsFunc:      componentsnfs.SetDefaultConfigs,
		setDefaultXIDRebootThresholdFunc:   componentsxid.SetDefaultRebootThreshold,

		nvmlInstance:       op.nvmlInstance,
		metricsStore:       op.metricsStore,
		componentsRegistry: op.componentsRegistry,
		processRunner:      process.NewExclusiveRunner(),

		components: cps,

		savePluginSpecsFunc: op.savePluginSpecsFunc,
		faultInjector:       op.faultInjector,

		enableAutoUpdate:   op.enableAutoUpdate,
		autoUpdateExitCode: op.autoUpdateExitCode,
	}

	// Initialize testable functions with default implementations
	s.timeAfterFunc = time.After
	s.timeSleepFunc = time.Sleep

	s.startReaderFunc = s.startReader
	s.startWriterFunc = s.startWriter
	s.checkServerHealthFunc = s.checkServerHealth

	s.reader = make(chan Body, 20)
	s.writer = make(chan Body, 20)
	s.closer = &closeOnce{closer: make(chan any)}
	go s.keepAlive()
	go s.serve()

	return s, nil
}

type Body struct {
	Data  []byte `json:"data,omitempty"`
	ReqID string `json:"req_id,omitempty"`
}

// drainReaderChannel removes any stale messages from the reader channel
// to prevent overflow when new connections start writing to it.
// These stale messages could be from:
// - Old reader goroutines that wrote messages before exiting
// - Messages that arrived during the cleanup period
func (s *Session) drainReaderChannel() {
	drained := 0
	for {
		select {
		case <-s.reader:
			drained++
		default:
			// No more messages to drain
			if drained > 0 {
				log.Logger.Warnw("drained stale messages from reader channel", "count", drained)
			}
			return
		}
	}
}

func (s *Session) keepAlive() {
	// Start the initial connection immediately
	firstConnection := true

	for {
		select {
		case <-s.ctx.Done():
			log.Logger.Debug("session keep alive: closing keep alive")
			return
		default:
			// CRITICAL: Reconnection delay to prevent race conditions
			//
			// Without this delay, rapid connection failures would create multiple
			// overlapping reader/writer goroutines that all write to the same channels,
			// causing "reader channel full" errors and making GPUd unresponsive.
			//
			// The 3-second delay ensures:
			// - Previous connections have time to fully clean up
			// - We don't overwhelm the control plane with rapid reconnection attempts
			// - Only one set of reader/writer goroutines exists at a time
			if !firstConnection {
				select {
				case <-s.ctx.Done():
					return
				case <-s.timeAfterFunc(3 * time.Second):
					log.Logger.Debug("session keep alive: attempting reconnection after delay")
				}
			}
			firstConnection = false

			readerExit := make(chan any)
			writerExit := make(chan any)

			// CLEANUP: Ensure previous connections are fully terminated
			//
			// This cleanup is essential to prevent the "reader channel full" bug:
			// - s.closer.Close() signals any existing reader/writer goroutines to exit
			// - The sleep gives them time to process the signal and clean up
			// - We drain stale messages to prevent them from blocking new connections
			//
			// Without this cleanup, old goroutines could still be writing to channels
			// when new ones start, causing race conditions and channel overflow
			if s.closer != nil {
				s.closer.Close()

				// Give old goroutines time to detect closer signal and exit
				s.timeSleepFunc(100 * time.Millisecond)

				// Drain any stale messages left in the reader channel
				s.drainReaderChannel()
			}

			s.closer = &closeOnce{closer: make(chan any)}
			ctx, cancel := context.WithCancel(s.ctx) // create local context derived from session context
			// DO NOT CHANGE OR REMOVE THIS COOKIE JAR, DEPEND ON IT FOR STICKY SESSION
			jar, _ := cookiejar.New(nil)

			log.Logger.Infow("session keep alive: checking server health")
			// DO NOT CHANGE OR REMOVE THIS SERVER HEALTH CHECK, DEPEND ON IT FOR STICKY SESSION
			if err := s.checkServerHealthFunc(ctx, jar); err != nil {
				log.Logger.Errorf("session keep alive: error checking server health: %v", err)
				cancel()
				continue
			}

			go s.startReaderFunc(ctx, readerExit, jar)
			go s.startWriterFunc(ctx, writerExit, jar)

			// CRITICAL: We must handle EITHER reader or writer exiting first to prevent deadlock
			//
			// Why we use select instead of waiting for reader then writer:
			// 1. Reader can fail first: EOF, network errors, decode errors
			// 2. Writer can fail first: connection broken during send, pipe closed
			// 3. If we always waited for reader first (<-readerExit), we'd deadlock if writer exits first
			//
			// How this prevents deadlock:
			// - Whichever goroutine exits first (reader OR writer) triggers the cleanup
			// - We immediately cancel the context to signal the other goroutine to exit
			// - Then we wait for the other goroutine to finish cleanup
			// - This ensures both goroutines are fully terminated before reconnecting
			//
			// This pattern guarantees:
			// - No deadlock: We handle both possible exit orders
			// - Clean shutdown: Both goroutines exit before we create new ones
			// - No goroutine leaks: Context cancellation ensures termination
			select {
			case <-readerExit:
				log.Logger.Debug("session reader: reader exited first")
				cancel()     // Signal writer to exit
				<-writerExit // Wait for writer cleanup
				log.Logger.Debug("session writer: writer exited after cancellation")
			case <-writerExit:
				log.Logger.Debug("session writer: writer exited first")
				cancel()     // Signal reader to exit
				<-readerExit // Wait for reader cleanup
				log.Logger.Debug("session reader: reader exited after cancellation")
			}
		}
	}
}

func createHTTPClient(jar *cookiejar.Jar) *http.Client {
	return &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:       30 * time.Second,
				KeepAlive:     30 * time.Second,
				FallbackDelay: 300 * time.Millisecond,
			}).DialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
			DisableKeepAlives:     true,
		},
	}
}

func createSessionRequest(ctx context.Context, epControlPlane, machineID, sessionType, token string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", epControlPlane+"/api/v1/session", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-GPUD-Machine-ID", machineID)
	req.Header.Set("X-GPUD-Session-Type", sessionType)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Depreciated headers
	req.Header.Set("machine_id", machineID)
	req.Header.Set("session_type", sessionType)
	req.Header.Set("token", token)

	return req, nil
}

func (s *Session) startWriter(ctx context.Context, writerExit chan any, jar *cookiejar.Jar) {
	pipeFinishCh := make(chan any)
	goroutineCloseCh := make(chan any)
	defer func() {
		close(goroutineCloseCh)
		s.closer.Close()
		<-pipeFinishCh
		close(writerExit)
	}()
	reader, writer := io.Pipe()
	go s.handleWriterPipe(writer, goroutineCloseCh, pipeFinishCh)

	req, err := createSessionRequest(ctx, s.epControlPlane, s.machineID, "write", s.token, reader)
	if err != nil {
		log.Logger.Warnw("session writer: error creating request", "error", err)
		return
	}

	client := createHTTPClient(jar)
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Warnw("session writer: error making request", "error", err)
		return
	}

	log.Logger.Debugw("session writer: http closed", "status", resp.Status, "statusCode", resp.StatusCode)
}

func (s *Session) handleWriterPipe(writer *io.PipeWriter, closec, finish chan any) {
	defer close(finish)
	defer writer.Close()

	log.Logger.Debugw("session writer: pipe handler started")
	for {
		select {
		case <-s.closer.Done():
			log.Logger.Debugw("session writer: session closed, closing pipe handler")
			return

		case <-closec:
			log.Logger.Debugw("session writer: request finished, closing pipe handler")
			return

		case body := <-s.writer:
			if err := s.writeBodyToPipe(writer, body); err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					return
				}
			}
		}
	}
}

func (s *Session) writeBodyToPipe(writer *io.PipeWriter, body Body) error {
	bytes, err := json.Marshal(body)
	if err != nil {
		log.Logger.Errorw("session writer: failed to marshal body", "error", err)
		return err
	}

	if _, err := writer.Write(bytes); err != nil {
		log.Logger.Errorw("session writer: failed to write to pipe", "error", err)
		return err
	}

	log.Logger.Debugw("session writer: body written to pipe")
	return nil
}

func (s *Session) startReader(ctx context.Context, readerExit chan any, jar *cookiejar.Jar) {
	goroutineCloseCh := make(chan any)
	pipeFinishCh := make(chan any)
	defer func() {
		close(goroutineCloseCh)
		s.closer.Close()
		<-pipeFinishCh
		close(readerExit)
	}()

	req, err := createSessionRequest(ctx, s.epControlPlane, s.machineID, "read", s.token, nil)
	if err != nil {
		log.Logger.Debugw("session reader: error creating request", "error", err)
		close(pipeFinishCh)
		return
	}

	client := createHTTPClient(jar)
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Warnw("session reader: error making request -- retrying", "error", err)
		close(pipeFinishCh)
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Logger.Warnw("session reader: request resp not ok -- retrying", "status", resp.Status, "statusCode", resp.StatusCode)
		close(pipeFinishCh)
		return
	}

	s.processReaderResponse(resp, goroutineCloseCh, pipeFinishCh)
}

func (s *Session) checkServerHealth(ctx context.Context, jar *cookiejar.Jar) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.epControlPlane+"/healthz", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.token))

	client := createHTTPClient(jar)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server health check failed: %s", resp.Status)
	}
	return nil
}

func (s *Session) setLastPackageTimestamp(t time.Time) {
	s.lastPackageTimestampMu.Lock()
	defer s.lastPackageTimestampMu.Unlock()
	s.lastPackageTimestamp = t
}

func (s *Session) getLastPackageTimestamp() time.Time {
	s.lastPackageTimestampMu.RLock()
	defer s.lastPackageTimestampMu.RUnlock()
	return s.lastPackageTimestamp
}

func (s *Session) processReaderResponse(resp *http.Response, goroutineCloseCh, pipeFinishCh chan any) {
	s.setLastPackageTimestamp(time.Now())

	decoder := json.NewDecoder(resp.Body)
	go s.handleReaderPipe(resp.Body, goroutineCloseCh, pipeFinishCh)

	for {
		var content Body
		if err := decoder.Decode(&content); err != nil {
			log.Logger.Errorw("session reader: error decoding response", "error", err)
			break
		}

		if !s.tryWriteToReader(content) {
			return
		}
	}
}

func (s *Session) tryWriteToReader(content Body) bool {
	s.auditLogger.Log(
		log.WithKind("Session"),
		log.WithAuditID(content.ReqID),
		log.WithMachineID(s.machineID),
		log.WithStage("RequestReceived"),
		log.WithRequestURI(s.epControlPlane+"/api/v1/session"),
		log.WithData(string(content.Data)),
	)

	select {
	case <-s.closer.Done():
		log.Logger.Debug("session reader: session closed, dropping message")
		return false

	case s.reader <- content:
		s.setLastPackageTimestamp(time.Now())
		log.Logger.Debug("session reader: request received and written to pipe")
		return true

	default:
		log.Logger.Errorw("session reader: reader channel full, dropping message -- WARNING: THIS CAN MAKE GPUd UNRESPONSIVE TO CONTROL PLANE REQUESTS (OFFLINE)")
		return true
	}
}

func (s *Session) handleReaderPipe(respBody io.ReadCloser, closec, finish chan any) {
	defer close(finish)

	log.Logger.Debugw("session reader: pipe handler started")

	threshold := 2 * time.Minute
	ticker := time.NewTicker(1 * time.Second)
	defer func() {
		respBody.Close()
		ticker.Stop()
	}()
	for {
		select {
		case <-s.closer.Done():
			log.Logger.Debugw("session reader: session closed, closing read pipe handler")
			return

		case <-closec:
			log.Logger.Debugw("session reader: request finished, closing read pipe handler")
			return

		case <-ticker.C:
			if time.Since(s.getLastPackageTimestamp()) > threshold {
				log.Logger.Debugw("session reader: exceed read wait timeout, closing read pipe handler")
				return
			}
		}
	}
}

func (s *Session) Stop() {
	select {
	case <-s.ctx.Done():
		return

	default:
		log.Logger.Debugw("closing session...")

		s.cancel()
		s.closer.Close()
		close(s.reader)
		close(s.writer)
	}
}
