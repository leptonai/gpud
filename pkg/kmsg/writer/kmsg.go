// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package writer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"

	"golang.org/x/sys/unix"

	"github.com/leptonai/gpud/pkg/log"
)

const DefaultDevKmsg = "/dev/kmsg"

func openKmsgForWrite(devFile string) (io.Writer, error) {
	kmsg, err := os.OpenFile(devFile, os.O_RDWR|unix.O_CLOEXEC|unix.O_NONBLOCK|unix.O_NOCTTY, 0o666)
	if err != nil {
		return nil, fmt.Errorf("failed to open %q: %w", devFile, err)
	}
	return kmsg, nil
}

// KmsgWriter defines the interface for writing kernel messages.
type KmsgWriter interface {
	// Write writes a kernel message to the kernel log.
	Write(msg *KernelMessage) error
}

func NewWriter(devFile string) KmsgWriter {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return &noOpWriter{}
	}

	if devFile == "" {
		devFile = DefaultDevKmsg
	}

	wr, err := openKmsgForWrite(devFile)
	if err != nil {
		log.Logger.Errorw("failed to open kmsg for write", "error", err)
		return &noOpWriter{}
	}
	return &kmsgWriter{wr: wr}
}

var _ KmsgWriter = &noOpWriter{}

type noOpWriter struct{}

func (w *noOpWriter) Write(_ *KernelMessage) error {
	return nil
}

var _ KmsgWriter = &kmsgWriter{}

type kmsgWriter struct {
	wr io.Writer
}

// Write implements io.Writer interface with priority support.
// https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
// Copied from https://github.com/siderolabs/go-kmsg/blob/main/writer.go.
func (w *kmsgWriter) Write(msg *KernelMessage) error {
	priority := msg.Priority.SyslogPriority()

	p := []byte(msg.Message)
	for len(p) > 0 { // split writes by `\n`, and limit each line to MaxPrintkRecordLength
		i := bytes.IndexByte(p, '\n')
		if i == -1 {
			i = len(p) - 1
		}

		line := p[:i+1]

		prioritizedLine := buildKmsgLine(priority, line)
		if len(prioritizedLine) > MaxPrintkRecordLength {
			prioritizedLine = append(prioritizedLine[:MaxPrintkRecordLength-4], []byte("...\n")...)
		}

		if _, err := w.wr.Write(prioritizedLine); err != nil {
			return err
		}

		p = p[i+1:]
	}

	return nil
}

func buildKmsgLine(priority int, line []byte) []byte {
	// remove trailing newline to add priority prefix, we'll add it back later
	trimmed := bytes.TrimSuffix(line, []byte{'\n'})

	// replace tab characters from multierror library error messages with spaces,
	// as tabs are not visible in the console
	trimmed = bytes.ReplaceAll(trimmed, []byte{'\t'}, []byte{' '})

	// format message with syslog priority: "<priority>message"
	msg := fmt.Sprintf("<%d>%s\n", priority, string(trimmed))
	return []byte(msg)
}
