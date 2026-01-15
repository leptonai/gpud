package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsnvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsnvidiainfinibanditypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	componentsnvidianvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	componentstemperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	"github.com/leptonai/gpud/pkg/config"
	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/process"
	sessionstates "github.com/leptonai/gpud/pkg/session/states"
	"github.com/leptonai/gpud/pkg/sqlite"
)

type Op struct {
	auditLogger         log.AuditLogger
	machineID           string
	pipeInterval        time.Duration
	enableAutoUpdate    bool
	autoUpdateExitCode  int
	skipUpdateConfig    bool
	componentsRegistry  components.Registry
	dataDir             string
	dbInMemory          bool
	nvmlInstance        nvidianvml.Instance
	metricsStore        pkgmetrics.Store
	savePluginSpecsFunc func(context.Context, pkgcustomplugins.Specs) (bool, error)
	faultInjector       pkgfaultinjector.Injector
	dbRW                *sql.DB
	dbRO                *sql.DB
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

// WithSkipUpdateConfig skips processing updateConfig session requests when true.
func WithSkipUpdateConfig(skip bool) OpOption {
	return func(op *Op) {
		op.skipUpdateConfig = skip
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

func WithDataDir(dataDir string) OpOption {
	return func(op *Op) {
		op.dataDir = dataDir
	}
}

func WithDBInMemory(dbInMemory bool) OpOption {
	return func(op *Op) {
		op.dbInMemory = dbInMemory
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

func WithDB(dbRW *sql.DB, dbRO *sql.DB) OpOption {
	return func(op *Op) {
		op.dbRW = dbRW
		op.dbRO = dbRO
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

	tokenMu    sync.RWMutex
	token      string
	dataDir    string
	dbInMemory bool

	dbRW *sql.DB
	dbRO *sql.DB

	createGossipRequestFunc func(machineID string, nvmlInstance nvidianvml.Instance) (*apiv1.GossipRequest, error)

	setDefaultIbExpectedPortStatesFunc     func(states componentsnvidiainfinibanditypes.ExpectedPortStates)
	setDefaultNVLinkExpectedLinkStatesFunc func(states componentsnvidianvlink.ExpectedLinkStates)
	setDefaultGPUCountsFunc                func(counts componentsnvidiagpucounts.ExpectedGPUCounts)
	setDefaultNFSGroupConfigsFunc          func(cfgs pkgnfschecker.Configs)
	setDefaultXIDRebootThresholdFunc       func(threshold componentsxid.RebootThreshold)
	setDefaultTemperatureThresholdsFunc    func(threshold componentstemperature.Thresholds)

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
	skipUpdateConfig    bool

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
	checkServerHealthFunc func(ctx context.Context, jar *cookiejar.Jar, token string) error
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

	dataDir, err := config.ResolveDataDir(op.dataDir)
	if err != nil {
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

		machineID:  op.machineID,
		token:      token,
		dataDir:    dataDir,
		dbInMemory: op.dbInMemory,

		tokenMu: sync.RWMutex{},
		dbRW:    op.dbRW,
		dbRO:    op.dbRO,

		createGossipRequestFunc: pkgmachineinfo.CreateGossipRequest,

		setDefaultIbExpectedPortStatesFunc:     componentsnvidiainfiniband.SetDefaultExpectedPortStates,
		setDefaultNVLinkExpectedLinkStatesFunc: componentsnvidianvlink.SetDefaultExpectedLinkStates,
		setDefaultGPUCountsFunc:                componentsnvidiagpucounts.SetDefaultExpectedGPUCounts,
		setDefaultNFSGroupConfigsFunc:          componentsnfs.SetDefaultConfigs,
		setDefaultXIDRebootThresholdFunc:       componentsxid.SetDefaultRebootThreshold,
		setDefaultTemperatureThresholdsFunc:    componentstemperature.SetDefaultMarginThreshold,

		nvmlInstance:       op.nvmlInstance,
		metricsStore:       op.metricsStore,
		componentsRegistry: op.componentsRegistry,
		processRunner:      process.NewExclusiveRunner(),

		components: cps,

		savePluginSpecsFunc: op.savePluginSpecsFunc,
		faultInjector:       op.faultInjector,
		skipUpdateConfig:    op.skipUpdateConfig,

		enableAutoUpdate:   op.enableAutoUpdate,
		autoUpdateExitCode: op.autoUpdateExitCode,
	}

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
	u, err := url.Parse(epControlPlane)
	if err != nil {
		return nil, err
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("no host in epControlPlane")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", epControlPlane+"/api/v1/session", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-GPUD-Machine-ID", machineID)
	req.Header.Set("X-GPUD-Session-Type", sessionType)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Origin", host)

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

	serverID := resp.Header.Get("X-GPUD-Server-ID")
	log.Logger.Infow("session writer: http closed", "status", resp.Status, "statusCode", resp.StatusCode, "serverID", serverID)
}

func (s *Session) handleWriterPipe(writer *io.PipeWriter, closec, finish chan any) {
	defer close(finish)
	defer func() {
		_ = writer.Close()
	}()

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

		// Persist 403 Forbidden errors to session_states table
		if resp.StatusCode == http.StatusForbidden {
			s.persistLoginFailure(ctx, resp)
		}

		close(pipeFinishCh)
		return
	}

	serverID := resp.Header.Get("X-GPUD-Server-ID")
	log.Logger.Infow("session reader got X-GPUD-Server-ID", "serverID", serverID)

	s.processReaderResponse(resp, goroutineCloseCh, pipeFinishCh)
}

func (s *Session) checkServerHealth(ctx context.Context, jar *cookiejar.Jar, token string) error {
	u, err := url.Parse(s.epControlPlane)
	if err != nil {
		return err
	}
	if strings.HasPrefix(u.Hostname(), "gpud-gateway.") {
		return nil
	}

	// TODO: we should remove the function once we migrate the session to gpud-gateway
	log.Logger.Infow("session keep alive: checking server health")
	req, err := http.NewRequestWithContext(ctx, "GET", s.epControlPlane+"/healthz", nil)
	if err != nil {
		return err
	}
	if token == "" {
		token = s.getToken()
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := createHTTPClient(jar)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

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

func (s *Session) setToken(token string) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	s.token = token
}

func (s *Session) getToken() string {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.token
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
		_ = respBody.Close()
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

func (s *Session) persistLoginFailure(ctx context.Context, resp *http.Response) {
	// Read response body to get error message
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Logger.Warnw("failed to read response body for login failure", "error", err)
		return
	}

	message := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	if len(message) > 500 {
		message = message[:500] // Truncate long messages
	}

	s.persistLoginStatus(ctx, false, message)
}

func (s *Session) persistLoginStatus(ctx context.Context, success bool, message string) {
	stateFile := config.StateFilePath(s.dataDir)

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		log.Logger.Warnw("failed to open state file for login status", "error", err)
		return
	}
	defer func() {
		_ = dbRW.Close()
	}()

	// Ensure table exists
	if err := sessionstates.CreateTable(ctx, dbRW); err != nil {
		log.Logger.Warnw("failed to create session_states table", "error", err)
		return
	}

	// Insert login status entry
	timestamp := time.Now().Unix()
	if err := sessionstates.Insert(ctx, dbRW, timestamp, success, message); err != nil {
		log.Logger.Warnw("failed to insert login status", "error", err)
		return
	}

	log.Logger.Infow("persisted login status entry", "success", success, "message", message, "timestamp", timestamp)
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
