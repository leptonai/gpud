/*
Copyright 2016 Euan Kemp

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file is a modified version of the following kmsg processing code:
// - https://github.com/util-linux/util-linux/blob/9c45b256adfc22edbf3783731b7be2b924c8de85/sys-utils/dmesg.c#L1512
// - https://github.com/euank/go-kmsg-parser

package kmsg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/dustin/go-humanize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/log"
)

var ErrWatcherAlreadyStarted = errors.New("watcher already started")

const streamPollInterval = 100 * time.Millisecond

type Watcher interface {
	Watch() (<-chan Message, error)
	Close() error
}

// Message is GPUd's kernel-record representation. Collection and parsing belong
// to health-validation; event interpretation and content deduplication stay here.
type Message struct {
	Timestamp      metav1.Time `json:"timestamp"`
	Priority       int         `json:"priority"`
	SequenceNumber int         `json:"sequence_number"`
	Message        string      `json:"message"`
}

func (m Message) DescribeTimestamp(since time.Time) string {
	return humanize.RelTime(m.Timestamp.Time, since, "ago", "from now")
}

// ReadAll collects a snapshot through the sensor and applies GPUd content dedup.
// A partial collection returns both the available messages and an error.
func ReadAll(ctx context.Context) ([]Message, error) {
	if err := checkKmsgPath(); err != nil {
		return nil, err
	}
	sensor, err := newSensor(nil)
	if err != nil {
		return nil, err
	}
	records, err := collectMessages(ctx, sensor)
	deduper := newDeduper(defaultCacheExpiration, defaultCachePurgeInterval)
	messages := make([]Message, 0, len(records))
	for _, record := range records {
		if deduper.addCache(record) == 1 {
			messages = append(messages, record)
		}
	}
	return messages, err
}

type stream interface {
	Read(context.Context) ([]kmsgsource.Message, error)
	Close() error
}

type watcher struct {
	sensor       collect.Sensor
	stream       stream
	ctx          context.Context
	cancel       context.CancelFunc
	closeOnce    sync.Once
	closeErr     error
	watchStarted atomic.Bool
	deduperOpts  []OpOption
}

// NewWatcher delegates native reading, boot identity, and parsing to the shared
// stream. The sensor emits complete records so component matchers can reconstruct
// multi-record events. The stream retains a bounded window, not a lossless queue.
func NewWatcher(opts ...OpOption) (Watcher, error) {
	if err := checkKmsgPath(); err != nil {
		return nil, err
	}
	source, err := kmsgsource.NewStream(0)
	if err != nil {
		return nil, err
	}
	return newWatcher(source, opts...)
}

func newWatcher(source stream, opts ...OpOption) (*watcher, error) {
	// Stream.Read returns a copy of its retained window. Consume each identity
	// once before sensor matching so unchanged windows do not rebuild observations.
	var cursor recordCursor
	sensor, err := newSensor(func(ctx context.Context) ([]kmsgsource.Message, error) {
		records, err := source.Read(ctx)
		fresh := records[:0]
		for _, record := range records {
			// Preserve invalid records for the sensor to report as collection failures.
			if record.BootID == "" || record.Sequence < 0 {
				fresh = append(fresh, record)
				continue
			}
			accept, gap := cursor.advance(recordID{bootID: record.BootID, sequence: record.Sequence})
			if gap != nil {
				log.Logger.Errorw("kmsg records lost before collection", "error", gap)
			}
			if accept {
				fresh = append(fresh, record)
			}
		}
		return fresh, err
	})
	if err != nil {
		_ = source.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &watcher{sensor: sensor, stream: source, ctx: ctx, cancel: cancel, deduperOpts: append([]OpOption(nil), opts...)}, nil
}

func checkKmsgPath() error {
	if path := strings.TrimSpace(os.Getenv("KMSG_FILE_PATH")); path != "" && path != "/dev/kmsg" {
		return fmt.Errorf("health-validation reads /dev/kmsg; KMSG_FILE_PATH=%q is unsupported", path)
	}
	return nil
}

func (w *watcher) Watch() (<-chan Message, error) {
	if !w.watchStarted.CompareAndSwap(false, true) {
		return nil, ErrWatcherAlreadyStarted
	}
	if err := w.ctx.Err(); err != nil {
		return nil, err
	}
	messages := make(chan Message, 2048)
	go func() {
		defer close(messages)
		defer func() { _ = w.Close() }()
		if err := w.follow(messages); err != nil && w.ctx.Err() == nil {
			log.Logger.Errorw("kmsg watcher error", "error", err)
		}
	}()
	return messages, nil
}

func (w *watcher) follow(messages chan<- Message) error {
	ticker := time.NewTicker(streamPollInterval)
	defer ticker.Stop()
	deduper := newDeduper(defaultCacheExpiration, defaultCachePurgeInterval, w.deduperOpts...)
	for {
		records, err := collectMessages(w.ctx, w.sensor)
		for _, record := range records {
			if deduper != nil && deduper.addCache(record) > 1 {
				continue
			}
			select {
			case messages <- record:
			case <-w.ctx.Done():
				return w.ctx.Err()
			}
		}
		if err != nil {
			return err
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *watcher) Close() error {
	w.closeOnce.Do(func() {
		w.cancel()
		w.closeErr = w.stream.Close()
	})
	return w.closeErr
}

// The source window is non-destructive. Advance before content deduplication so
// repeated bodies do not look like missing sequence numbers on the next poll.
type recordCursor struct{ last recordID }

func (c *recordCursor) advance(id recordID) (bool, error) {
	if c.last.bootID == id.bootID && id.sequence <= c.last.sequence {
		return false, nil
	}
	var err error
	if c.last.bootID == id.bootID && id.sequence-c.last.sequence > 1 {
		err = fmt.Errorf("boot %s: missing kernel sequences after %d and before %d", id.bootID, c.last.sequence, id.sequence)
	}
	c.last = id
	return true, err
}
