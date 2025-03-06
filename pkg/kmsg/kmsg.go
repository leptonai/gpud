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
	"io"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/v4/host"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/log"
)

// Message represents a given kmsg logline, including its timestamp (as
// calculated based on offset from boot time), its possibly multi-line body,
// and so on. More information about these mssages may be found here:
// https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
type Message struct {
	Timestamp      metav1.Time `json:"timestamp"`
	Priority       int         `json:"priority"`
	SequenceNumber int         `json:"sequence_number"`
	Message        string      `json:"message"`
}

func (m Message) DescribeTimestamp(since time.Time) string {
	return humanize.RelTime(m.Timestamp.Time, since, "ago", "from now")
}

// ReadAll reads all messages from the kmsg file, with no follow mode.
func ReadAll(ctx context.Context) ([]Message, error) {
	kmsgFile, err := os.Open("/dev/kmsg")
	if err != nil {
		return nil, err
	}
	defer kmsgFile.Close()

	// upstream "github.com/euank/go-kmsg-parser" uses "syscall.Sysinfo_t"
	// which breaks darwin builds
	// use multi-platform library "github.com/shirou/gopsutil/v4/host"
	// to support darwin builds
	cctx, ccancel := context.WithTimeout(ctx, 15*time.Second)
	bt, err := host.BootTimeWithContext(cctx)
	ccancel()
	if err != nil {
		return nil, err
	}
	bootTime := time.Unix(int64(bt), 0)

	return readAll(kmsgFile, bootTime)
}

// any value >= PRINTK_MESSAGE_MAX (which is defined as 2048) is fine
// "dmesg" uses 2048
// ref. https://github.com/util-linux/util-linux/blob/9c45b256adfc22edbf3783731b7be2b924c8de85/sys-utils/dmesg.c#L212-L217
const readBufferSize = 8192

// readAll reads all messages from the kmsg file, with no follow mode.
func readAll(kmsgFile *os.File, bootTime time.Time) ([]Message, error) {
	rawReader, err := kmsgFile.SyscallConn()
	if err != nil {
		return nil, err
	}

	// we're forced to put the fd into non-blocking mode to be able to detect the
	// end of the buffer, but to not use go's built-in epoll
	if ctrlErr := rawReader.Control(func(fd uintptr) {
		err = syscall.SetNonblock(int(fd), true)
	}); ctrlErr != nil {
		return nil, fmt.Errorf("error calling control on kmsg reader: %w", ctrlErr)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to set nonblocking on fd: %w", err)
	}

	msgs := make([]Message, 0)
	buf := make([]byte, readBufferSize)
	for {
		// Each read call gives us one full message.
		// https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
		var err error
		var n int
		readErr := rawReader.Read(func(fd uintptr) bool {
			n, err = syscall.Read(int(fd), buf)
			return true
		})
		if readErr != nil && err == nil {
			err = readErr
		}

		switch {
		case err == nil:
			// no-op

		case errors.Is(err, syscall.EPIPE):
			// error that indicates "continuation" of the ring buffer
			continue

		case errors.Is(err, syscall.EAGAIN):
			// end of ring buffer in non-follow mode
			return msgs, nil

		default:
			return nil, fmt.Errorf("unexpected kmsg reading error: %w", err)
		}

		line := string(buf[:n])
		if len(line) == 0 {
			continue
		}

		msg, err := parseLine(bootTime, line)
		if err != nil {
			return nil, fmt.Errorf("malformed kmsg message: %w (%q)", err, line)
		}
		msgs = append(msgs, *msg)
	}
}

type Watcher interface {
	// Watch reads from kmsg and provides a channel of messages.
	// Watch will always close the provided channel before returning.
	// Watch may be canceled by calling 'Close' on the parser.
	//
	// The caller should drain the channel after calling 'Close'.
	Watch(chan<- Message) error
	Close() error
}

type watcher struct {
	kmsgFile *os.File
	bootTime time.Time

	// set to true when the watcher is started
	// used to prevent redundant reads on kmsg file
	watchStarted atomic.Bool
}

// Creates a new watcher that will read from /dev/kmsg.
func NewWatcher() (Watcher, error) {
	kmsgFile, err := os.Open("/dev/kmsg")
	if err != nil {
		return nil, err
	}

	// upstream "github.com/euank/go-kmsg-parser" uses "syscall.Sysinfo_t"
	// which breaks darwin builds
	// use multi-platform library "github.com/shirou/gopsutil/v4/host"
	// to support darwin builds
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	bt, err := host.BootTimeWithContext(ctx)
	cancel()
	if err != nil {
		return nil, err
	}
	bootTime := time.Unix(int64(bt), 0)

	return &watcher{
		kmsgFile: kmsgFile,
		bootTime: bootTime,
	}, nil
}

var ErrWatcherAlreadyStarted = errors.New("watcher already started")

func (w *watcher) errIfStarted() error {
	if w.watchStarted.CompareAndSwap(false, true) {
		// has not started yet, thus set to started
		return nil
	}
	return ErrWatcherAlreadyStarted
}

func (w *watcher) Watch(msgs chan<- Message) error {
	if err := w.errIfStarted(); err != nil {
		return err
	}
	return readFollow(w.kmsgFile, w.bootTime, msgs)
}

func (w *watcher) Close() error {
	return w.kmsgFile.Close()
}

// readFollow reads messages from the kmsg file, with follow mode,
// meaning it will continue to read the file as new messages are written to it.
func readFollow(kmsgFile *os.File, bootTime time.Time, msgs chan<- Message) error {
	defer close(msgs)

	buf := make([]byte, readBufferSize)
	for {
		// Each read call gives us one full message.
		// https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
		n, err := kmsgFile.Read(buf)
		switch {
		case err == nil:
			// no-op

		case errors.Is(err, syscall.EPIPE):
			log.Logger.Warnw("kmsg pipe error (short read)")
			continue

		case errors.Is(err, io.EOF),
			errors.Is(err, os.ErrClosed):
			// someone closed the kmsg file
			return nil

		default:
			return fmt.Errorf("unexpected kmsg reading error: %w", err)
		}

		line := string(buf[:n])
		if len(line) == 0 {
			continue
		}

		msg, err := parseLine(bootTime, line)
		if err != nil {
			return fmt.Errorf("malformed kmsg message: %w (%q)", err, line)
		}
		msgs <- *msg
	}
}

func parseLine(bootTime time.Time, line string) (*Message, error) {
	// Format:
	//   PRIORITY,SEQUENCE_NUM,TIMESTAMP,-;MESSAGE
	parts := strings.SplitN(line, ";", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid kmsg; must contain a ';'")
	}

	metadata, message := parts[0], parts[1]

	metadataParts := strings.Split(metadata, ",")
	if len(metadataParts) < 3 {
		return nil, fmt.Errorf("invalid kmsg: must contain at least 3 ',' separated pieces at the start")
	}

	priority, sequence, timestamp := metadataParts[0], metadataParts[1], metadataParts[2]

	prioNum, err := strconv.Atoi(priority)
	if err != nil {
		return nil, fmt.Errorf("could not parse %q as priority: %v", priority, err)
	}

	sequenceNum, err := strconv.Atoi(sequence)
	if err != nil {
		return nil, fmt.Errorf("could not parse %q as sequence number: %v", priority, err)
	}

	timestampUsFromBoot, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("could not parse %q as timestamp: %v", priority, err)
	}
	// timestamp is offset in microsecond from boottime.
	msgTime := bootTime.Add(time.Duration(timestampUsFromBoot) * time.Microsecond)

	return &Message{
		Priority:       prioNum,
		SequenceNumber: sequenceNum,
		Timestamp:      metav1.NewTime(msgTime),
		Message:        message,
	}, nil
}
