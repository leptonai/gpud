package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/leptonai/gpud/log"
)

type Session struct {
	machineID    string
	endpoint     string
	serverClosed bool
	components   []string

	writer         chan Body
	writerCloseCh  chan bool
	writerClosedCh chan bool

	reader         chan Body
	readerCloseCh  chan bool
	readerClosedCh chan bool
}

func NewSession(endpoint string, machineID string) *Session {
	s := &Session{
		endpoint:  endpoint,
		machineID: machineID,
	}
	s.reader = make(chan Body, 20)
	s.writer = make(chan Body, 20)
	s.writerCloseCh = make(chan bool, 2)
	s.writerClosedCh = make(chan bool)
	s.readerCloseCh = make(chan bool, 2)
	s.readerClosedCh = make(chan bool)
	s.keepAlive()
	s.serve()
	return s
}

type Body struct {
	Data  []byte `json:"data,omitempty"`
	ReqID string `json:"req_id,omitempty"`
}

func (s *Session) keepAlive() {
	s.startWriter()
	s.startReader()
}

func (s *Session) startWriter() {
	go func() {
		for {
			if s.serverClosed {
				log.Logger.Debugf("session writer: session closed, closing writer...")
				if s.writerClosedCh != nil {
					s.writerClosedCh <- true
					log.Logger.Debugf("session writer: writer chan closed.")
				}
				return
			}
			serverUrl := fmt.Sprintf("https://%s/api/v1/session", s.endpoint)

			reader, writer := io.Pipe()
			req, err := http.NewRequest("POST", serverUrl, reader)
			if err != nil {
				log.Logger.Errorf("session writer: error creating request: %v, retrying in 3s...", err)
				time.Sleep(3 * time.Second)
				continue
			}
			req.Header.Set("machine_id", s.machineID)
			req.Header.Set("session_type", "write")

			log.Logger.Debugf("session writer: created")
			goroutineCloseCh := make(chan any)
			go func() {
				for {
					select {
					case <-s.writerCloseCh:
						writer.Close()
					case <-goroutineCloseCh:
						return
					case body := <-s.writer:
						bytes, err := json.Marshal(body)
						if err != nil {
							log.Logger.Errorf("session writer: failed to marshal body: %v", err)
							continue
						}
						if _, err := writer.Write(bytes); err != nil {
							log.Logger.Errorf("session writer: failed to write to pipe: %v", err)
							continue
						}
					}
				}
			}()

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				log.Logger.Errorf("session writer: error making request: %v, retrying", err)
				time.Sleep(3 * time.Second)
				close(goroutineCloseCh)
				continue
			}
			log.Logger.Debugf("session writer: unexpected closed, resp: %v %v, reconnecting...", resp.Status, resp.StatusCode)
			close(goroutineCloseCh)
			time.Sleep(3 * time.Second)
		}
	}()
}

func (s *Session) startReader() {
	go func() {
		for {
			if s.serverClosed {
				log.Logger.Debugf("session reader: session closed, closing reader...")
				if s.readerClosedCh != nil {
					s.readerClosedCh <- true
					log.Logger.Debugf("session reader: read chan closed.")
				}
				return
			}
			serverUrl := fmt.Sprintf("https://%s/api/v1/session", s.endpoint)

			req, err := http.NewRequest("POST", serverUrl, nil)
			if err != nil {
				log.Logger.Errorf("session reader: error creating request: %v, retrying", err)
				time.Sleep(3 * time.Second)
				continue
			}
			req.Header.Set("machine_id", s.machineID)
			req.Header.Set("session_type", "read")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				log.Logger.Errorf("session reader: error making request: %v, retrying", err)
				time.Sleep(3 * time.Second)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				log.Logger.Errorf("session reader: error making request: %v %v, retrying", resp.StatusCode, resp.Status)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Logger.Debugf("session reader: reader channel created!")
			goroutineCloseCh := make(chan any)
			go func() {
				for {
					select {
					case <-goroutineCloseCh:
						return
					case <-s.readerCloseCh:
						resp.Body.Close()
					}
				}
			}()
			decoder := json.NewDecoder(resp.Body)
			for {
				var content Body
				err = decoder.Decode(&content)
				if err != nil {
					if !errors.Is(err, io.EOF) {
						fmt.Println("Error reading response:", err)
					}
					s.writerCloseCh <- true
					time.Sleep(3 * time.Second)
					break
				}
				s.reader <- content
			}
			close(goroutineCloseCh)
			time.Sleep(3 * time.Second)
		}
	}()
}

func (s *Session) Stop() {
	s.serverClosed = true
	s.writerCloseCh <- true
	s.readerCloseCh <- true
	log.Logger.Debugf("waiting for writer and reader to finish connection...")
	<-s.writerClosedCh
	<-s.readerClosedCh
	close(s.reader)
	close(s.writer)
}
