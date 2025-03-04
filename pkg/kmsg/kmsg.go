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

// This file is a modified version of the kmsg package
// https://github.com/euank/go-kmsg-parser.

package kmsg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/host"
)

// Message represents a given kmsg logline, including its timestamp (as
// calculated based on offset from boot time), its possibly multi-line body,
// and so on. More information about these mssages may be found here:
// https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
type Message struct {
	Priority       int
	SequenceNumber int
	Timestamp      time.Time
	Message        string
}

func ReadAll(ctx context.Context) ([]Message, error) {
	kmsgFile, err := os.Open("/dev/kmsg")
	if err != nil {
		return nil, err
	}
	defer kmsgFile.Close()

	cctx, ccancel := context.WithTimeout(ctx, 15*time.Second)
	bt, err := host.BootTimeWithContext(cctx)
	ccancel()
	if err != nil {
		return nil, err
	}
	bootTime := time.Unix(int64(bt), 0)

	return readAll(kmsgFile, bootTime)
}

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
	buf := make([]byte, 8192)
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
		case errors.Is(err, syscall.EPIPE):
			continue

		case errors.Is(err, syscall.EAGAIN):
			// end of ring buffer in nofollow mode, we're done
			return msgs, nil

		default:
			return nil, fmt.Errorf("unexpected kmsg reading error: %w", err)
		}

		line := string(buf[:n])
		if len(line) == 0 {
			continue
		}

		msg, err := parseMessage(bootTime, line)
		if err != nil {
			return nil, fmt.Errorf("malformed kmsg message: %w (%q)", err, line)
		}
		msgs = append(msgs, *msg)
	}
}

func parseMessage(bootTime time.Time, line string) (*Message, error) {
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
		Timestamp:      msgTime,
		Message:        message,
	}, nil
}
