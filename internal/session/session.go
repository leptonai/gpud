package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
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

	writer         chan Body
	writerCloseCh  chan bool
	writerClosedCh chan bool

	reader         chan Body
	readerCloseCh  chan bool
	readerClosedCh chan bool

	enableAutoUpdate   bool
	autoUpdateExitCode int
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
	s.writerCloseCh = make(chan bool, 2)
	s.writerClosedCh = make(chan bool)
	s.readerCloseCh = make(chan bool, 2)
	s.readerClosedCh = make(chan bool)
	s.keepAlive()
	go s.serve()

	return s, nil
}

type Body struct {
	Data  []byte `json:"data,omitempty"`
	ReqID string `json:"req_id,omitempty"`
}

func (s *Session) keepAlive() {
	go s.startWriter()
	go s.startReader()
}

func (s *Session) startWriter() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			log.Logger.Debug("session writer: closing writer")
			s.writerClosedCh <- true
			log.Logger.Debug("session writer: closed writer")
			return

		case <-ticker.C:
			ticker.Reset(s.pipeInterval)
		}

		reader, writer := io.Pipe()
		goroutineCloseCh := make(chan struct{})
		go s.handleWriterPipe(writer, goroutineCloseCh)

		req, err := http.NewRequestWithContext(s.ctx, "POST", s.endpoint, reader)
		if err != nil {
			log.Logger.Debugf("session writer: error creating request: %v, retrying in 3s...", err)
			close(goroutineCloseCh)
			continue
		}
		req.Header.Set("machine_id", s.machineID)
		req.Header.Set("session_type", "write")

		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout: 30 * time.Second,
					KeepAliveConfig: net.KeepAliveConfig{
						Enable:   true,
						Idle:     10 * time.Second,
						Interval: 15 * time.Second,
						Count:    3,
					},
					FallbackDelay: 300 * time.Millisecond,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          10,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Logger.Debugf("session writer: error making request: %v, retrying", err)
			close(goroutineCloseCh)
			continue
		}

		log.Logger.Debugf("session writer: unexpected closed, resp: %v %v, reconnecting...", resp.Status, resp.StatusCode)
		close(goroutineCloseCh)
	}
}

func (s *Session) handleWriterPipe(writer *io.PipeWriter, closec <-chan struct{}) {
	defer writer.Close()
	log.Logger.Debug("session writer pipe handler started")
	for {
		select {
		case <-s.writerCloseCh:
			log.Logger.Debug("session writer closed")
			return

		case <-closec:
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
		}
	}
}

func (s *Session) startReader() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			log.Logger.Debug("session reader: closing reader")
			s.readerClosedCh <- true
			log.Logger.Debug("session reader: closed reader")
			return

		case <-ticker.C:
			ticker.Reset(s.pipeInterval)
		}

		req, err := http.NewRequestWithContext(s.ctx, "POST", s.endpoint, nil)
		if err != nil {
			log.Logger.Debugf("session reader: error creating request: %v, retrying", err)
			continue
		}
		req.Header.Set("machine_id", s.machineID)
		req.Header.Set("session_type", "read")

		client := &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout: 30 * time.Second,
					KeepAliveConfig: net.KeepAliveConfig{
						Enable:   true,
						Idle:     10 * time.Second,
						Interval: 15 * time.Second,
						Count:    3,
					},
					FallbackDelay: 300 * time.Millisecond,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          10,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Logger.Debugf("session reader: error making request: %v, retrying", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			log.Logger.Debugf("session reader: error making request: %v %v, retrying", resp.StatusCode, resp.Status)
			continue
		}

		goroutineCloseCh := make(chan any)
		go func() {
			log.Logger.Debug("session reader created")
			for {
				select {
				case <-goroutineCloseCh:
					return
				case <-s.readerCloseCh:
					log.Logger.Debug("session reader closed")
					resp.Body.Close()
					return
				}
			}
		}()

		lastPackageTimestamp := &time.Time{}
		decoder := json.NewDecoder(resp.Body)
		keepAliveCh := make(chan any)
		go readerKeepAlive(resp.Body, lastPackageTimestamp, keepAliveCh)
		for {
			var content Body
			err = decoder.Decode(&content)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					log.Logger.Debugf("Error reading response: %v", err)
				}

				s.writerCloseCh <- true
				break
			}
			*lastPackageTimestamp = time.Now()
			select {
			case s.reader <- content:
			default:
				log.Logger.Errorw("session reader: reader channel full, dropping message")
			}
		}
		close(keepAliveCh)
		close(goroutineCloseCh)
	}
}

func readerKeepAlive(respBody io.ReadCloser, lastPackageTimestamp *time.Time, closeCh chan any) {
	threshold := 2 * time.Minute
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if time.Since(*lastPackageTimestamp) > threshold {
				respBody.Close()
				return
			}
		case <-closeCh:
			return
		}
	}
}

func (s *Session) Stop() {
	s.cancel()

	s.writerCloseCh <- true
	s.readerCloseCh <- true

	log.Logger.Debug("waiting for writer and reader to finish connection...")
	<-s.writerClosedCh
	<-s.readerClosedCh

	close(s.reader)
	close(s.writer)
}
