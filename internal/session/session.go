package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/log"
)

type Op struct {
	machineID          string
	pipeInterval       time.Duration
	enableAutoUpdate   bool
	autoUpdateExitCode int
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
	endpoint  string

	components []string

	closer *closeOnce

	writer chan Body
	reader chan Body

	enableAutoUpdate   bool
	autoUpdateExitCode int
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

func NewSession(ctx context.Context, endpoint string, opts ...OpOption) (*Session, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	cps := make([]string, 0)
	allComponents := components.GetAllComponents()
	for key := range allComponents {
		cps = append(cps, key)
	}

	cctx, ccancel := context.WithCancel(ctx)
	s := &Session{
		ctx:    cctx,
		cancel: ccancel,

		pipeInterval: op.pipeInterval,

		endpoint:  endpoint,
		machineID: op.machineID,

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
			go s.startReader(readerExit)
			go s.startWriter(writerExit)
			<-readerExit
			log.Logger.Debug("session reader: reader exited")
			<-writerExit
			log.Logger.Debug("session writer: writer exited")
		}
	}
}

func (s *Session) startWriter(writerExit chan any) {
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

	req, err := http.NewRequestWithContext(s.ctx, "POST", s.endpoint, reader)
	if err != nil {
		log.Logger.Debugf("session writer: error creating request: %v", err)
		return
	}
	req.Header.Set("machine_id", s.machineID)
	req.Header.Set("session_type", "write")

	client := &http.Client{
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
			bytes, err := json.Marshal(body)
			if err != nil {
				log.Logger.Errorf("session writer: failed to marshal body: %v", err)
				continue
			}
			if _, err := writer.Write(bytes); err != nil {
				log.Logger.Errorf("session writer: failed to write to pipe: %v", err)
				if errors.Is(err, io.ErrClosedPipe) {
					return
				}
			}
			log.Logger.Debug("session writer: body written to pipe")
		}
	}
}

func (s *Session) startReader(readerExit chan any) {
	goroutineCloseCh := make(chan any)
	pipeFinishCh := make(chan any)
	defer func() {
		close(goroutineCloseCh)
		s.closer.Close()
		<-pipeFinishCh
		close(readerExit)
	}()
	req, err := http.NewRequestWithContext(s.ctx, "POST", s.endpoint, nil)
	if err != nil {
		log.Logger.Debugf("session reader: error creating request: %v", err)
		return
	}
	req.Header.Set("machine_id", s.machineID)
	req.Header.Set("session_type", "read")

	client := &http.Client{
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
	resp, err := client.Do(req)
	if err != nil {
		log.Logger.Debugf("session reader: error making request: %v, retrying", err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Logger.Debugf("session reader: request resp not ok: %v %v, retrying", resp.StatusCode, resp.Status)
		return
	}

	lastPackageTimestamp := &time.Time{}
	*lastPackageTimestamp = time.Now()
	decoder := json.NewDecoder(resp.Body)
	go s.handleReaderPipe(resp.Body, lastPackageTimestamp, goroutineCloseCh, pipeFinishCh)
	for {
		var content Body
		err = decoder.Decode(&content)
		if err != nil {
			log.Logger.Errorf("session reader: error decoding response: %v", err)
			break
		}
		select {
		case <-s.closer.Done():
			log.Logger.Debug("session reader: session closed, dropping message")
			return
		case s.reader <- content:
			*lastPackageTimestamp = time.Now()
			log.Logger.Debug("session reader: request received and written to pipe")
		default:
			log.Logger.Errorw("session reader: reader channel full, dropping message")
		}
	}
}

func (s *Session) handleReaderPipe(respBody io.ReadCloser, lastPackageTimestamp *time.Time, closec, finish chan any) {
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
			if time.Since(*lastPackageTimestamp) > threshold {
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
