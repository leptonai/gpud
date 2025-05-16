package session

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	"github.com/leptonai/gpud/pkg/process"
)

type Op struct {
	machineID          string
	pipeInterval       time.Duration
	enableAutoUpdate   bool
	autoUpdateExitCode int
	componentsRegistry components.Registry
	metricsStore       pkgmetrics.Store
}

type OpOption func(*Op)

var ErrAutoUpdateDisabledButExitCodeSet = errors.New("auto update is disabled but auto update by exit code is set")

func (op *Op) applyOpts(opts []OpOption) error {
	op.autoUpdateExitCode = -1

	for _, opt := range opts {
		opt(op)
	}

	if !op.enableAutoUpdate && op.autoUpdateExitCode != -1 {
		return ErrAutoUpdateDisabledButExitCodeSet
	}

	return nil
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

func WithMetricsStore(metricsStore pkgmetrics.Store) OpOption {
	return func(op *Op) {
		op.metricsStore = metricsStore
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

	machineID string

	// epLocalGPUdServer is the endpoint of the local GPUd server
	epLocalGPUdServer string
	// epControlPlane is the endpoint of the control plane
	epControlPlane string

	token string

	metricsStore       pkgmetrics.Store
	componentsRegistry components.Registry
	processRunner      process.Runner

	components []string

	closer *closeOnce

	writer chan Body
	reader chan Body

	enableAutoUpdate   bool
	autoUpdateExitCode int

	lastPackageTimestampMu sync.RWMutex
	lastPackageTimestamp   time.Time
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

		epLocalGPUdServer: epLocalGPUdServer,
		epControlPlane:    epControlPlane,

		machineID: op.machineID,
		token:     token,

		metricsStore:       op.metricsStore,
		componentsRegistry: op.componentsRegistry,
		processRunner:      process.NewExclusiveRunner(),

		components: cps,

		enableAutoUpdate:   op.enableAutoUpdate,
		autoUpdateExitCode: op.autoUpdateExitCode,
	}

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

func (s *Session) keepAlive() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			log.Logger.Debug("session keep alive: closing keep alive")
			return
		case <-ticker.C:
			readerExit := make(chan any)
			writerExit := make(chan any)
			s.closer = &closeOnce{closer: make(chan any)}
			ctx, cancel := context.WithCancel(context.Background()) // create local context for each session
			jar, _ := cookiejar.New(nil)

			log.Logger.Infow("session keep alive: checking server health")
			if err := s.checkServerHealth(ctx, jar); err != nil {
				log.Logger.Errorf("session keep alive: error checking server health: %v", err)
				cancel()
				continue
			}

			go s.startReader(ctx, readerExit, jar)
			go s.startWriter(ctx, writerExit, jar)
			<-readerExit
			log.Logger.Debug("session reader: reader exited")
			cancel()
			<-writerExit
			log.Logger.Debug("session writer: writer exited")
			cancel()
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
		log.Logger.Debugf("session writer: error creating request: %v", err)
		return
	}

	client := createHTTPClient(jar)
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Debugf("session writer: error making request: %v", err)
		return
	}
	log.Logger.Debugf("session writer: http closed, resp: %v %v", resp.Status, resp.StatusCode)
}

func (s *Session) handleWriterPipe(writer *io.PipeWriter, closec, finish chan any) {
	defer close(finish)
	defer writer.Close()
	log.Logger.Debug("session writer: pipe handler started")
	for {
		select {
		case <-s.closer.Done():
			log.Logger.Debug("session writer: session closed, closing pipe handler")
			return
		case <-closec:
			log.Logger.Debug("session writer: request finished, closing pipe handler")
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
		log.Logger.Errorf("session writer: failed to marshal body: %v", err)
		return err
	}
	if _, err := writer.Write(bytes); err != nil {
		log.Logger.Errorf("session writer: failed to write to pipe: %v", err)
		return err
	}
	log.Logger.Debug("session writer: body written to pipe")
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
		log.Logger.Debugf("session reader: error creating request: %v", err)
		close(pipeFinishCh)
		return
	}

	client := createHTTPClient(jar)
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Debugf("session reader: error making request: %v, retrying", err)
		close(pipeFinishCh)
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Logger.Debugf("session reader: request resp not ok: %v %v, retrying", resp.StatusCode, resp.Status)
		close(pipeFinishCh)
		return
	}

	s.processReaderResponse(resp, goroutineCloseCh, pipeFinishCh)
}

func (s *Session) checkServerHealth(ctx context.Context, jar *cookiejar.Jar) error {
	req, err := http.NewRequestWithContext(ctx, "GET", s.epLocalGPUdServer+"/healthz", nil)
	if err != nil {
		return err
	}

	client := createHTTPClient(jar)
	tr := client.Transport.(*http.Transport)
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

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
			log.Logger.Errorf("session reader: error decoding response: %v", err)
			break
		}
		if !s.tryWriteToReader(content) {
			return
		}
	}
}

func (s *Session) tryWriteToReader(content Body) bool {
	select {
	case <-s.closer.Done():
		log.Logger.Debug("session reader: session closed, dropping message")
		return false
	case s.reader <- content:
		s.setLastPackageTimestamp(time.Now())
		log.Logger.Debug("session reader: request received and written to pipe")
		return true
	default:
		log.Logger.Errorw("session reader: reader channel full, dropping message")
		return true
	}
}

func (s *Session) handleReaderPipe(respBody io.ReadCloser, closec, finish chan any) {
	defer close(finish)
	log.Logger.Debug("session reader: pipe handler started")
	threshold := 2 * time.Minute
	ticker := time.NewTicker(1 * time.Second)
	defer func() {
		respBody.Close()
		ticker.Stop()
	}()
	for {
		select {
		case <-s.closer.Done():
			log.Logger.Debug("session reader: session closed, closing read pipe handler")
			return
		case <-closec:
			log.Logger.Debug("session reader: request finished, closing read pipe handler")
			return
		case <-ticker.C:
			if time.Since(s.getLastPackageTimestamp()) > threshold {
				log.Logger.Debugf("session reader: exceed read wait timeout, closing read pipe handler")
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
		log.Logger.Debug("closing session...")
		s.cancel()
		s.closer.Close()
		close(s.reader)
		close(s.writer)
	}
}
