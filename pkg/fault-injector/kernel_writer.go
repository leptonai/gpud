// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package faultinjector

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	kmsg "github.com/siderolabs/go-kmsg"

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

// Write applies GPUd's requested priority; the imported writer handles printk
// line limits, tab normalization, truncation, and write errors.
func (w *kmsgWriter) Write(msg *KernelMessage) error {
	writer := kmsg.Writer{KmsgWriter: w.wr}
	for line := range strings.SplitAfterSeq(msg.Message, "\n") {
		if line == "" {
			continue
		}
		data := fmt.Sprintf("<%d>%s\n", msg.Priority.SyslogPriority(), strings.TrimSuffix(line, "\n"))
		if _, err := writer.Write([]byte(data)); err != nil {
			return err
		}
	}
	return nil
}
