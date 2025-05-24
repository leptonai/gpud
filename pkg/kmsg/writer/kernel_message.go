package writer

import (
	"fmt"
	"os"
	"runtime"
)

// MaxPrintkRecordLength is the maximum length of a printk record.
// ref.
// "PRINTKRB_RECORD_MAX" in https://github.com/torvalds/linux/blob/94305e83eccb3120c921cd3a015cd74731140bac/kernel/printk/internal.h#L52
// "PRINTK_PREFIX_MAX" in https://github.com/torvalds/linux/blob/94305e83eccb3120c921cd3a015cd74731140bac/kernel/printk/internal.h#L40C9-L40C26
const MaxPrintkRecordLength = 1024 - 48

// KmsgWriter defines the interface for writing kernel messages.
type KmsgWriter interface {
	// Write writes a kernel message to the kernel log.
	Write(msg *KernelMessage) error
}

// KernelMessage represents a kernel message.
type KernelMessage struct {
	// Priority is the priority of the kernel message.
	// ref. https://github.com/torvalds/linux/blob/master/tools/include/linux/kern_levels.h#L8-L15
	Priority string `json:"priority"`
	// Message is the message of the kernel message.
	Message string `json:"message"`
}

// Validate validates the kernel message.
func (m *KernelMessage) Validate() error {
	if len(m.Message) > MaxPrintkRecordLength {
		return fmt.Errorf("message length exceeds the maximum length of %d", MaxPrintkRecordLength)
	}
	m.Priority = ConvertKernelMessagePriority(m.Priority)
	return nil
}

// ref. https://github.com/torvalds/linux/blob/master/tools/include/linux/kern_levels.h#L8-L15
func ConvertKernelMessagePriority(s string) string {
	switch s {
	case "KERN_EMERG", "kern.emerg":
		return "KERN_EMERG"
	case "KERN_ALERT", "kern.alert":
		return "KERN_ALERT"
	case "KERN_CRIT", "kern.crit":
		return "KERN_CRIT"
	case "KERN_ERR", "kern.err":
		return "KERN_ERR"
	case "KERN_WARNING", "kern.warning", "kern.warn":
		return "KERN_WARNING"
	case "KERN_NOTICE", "kern.notice":
		return "KERN_NOTICE"
	case "KERN_INFO", "kern.info":
		return "KERN_INFO"
	case "KERN_DEBUG", "kern.debug":
		return "KERN_DEBUG"
	case "KERN_DEFAULT", "kern.default":
		return "KERN_DEFAULT"
	default: // unknown priority, default to KERN_INFO
		return "KERN_INFO"
	}
}

func NewKmsgWriterWithDummyDevice() KmsgWriter {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return &noOpKmsgWriter{}
	}

	return &kmsgWriterWithDummyDevice{}
}

// kmsgWriterWithDummyDevice is a kernel message writer that writes to a dummy device.
type kmsgWriterWithDummyDevice struct {
}

func (w *kmsgWriterWithDummyDevice) Write(msg *KernelMessage) error {
	if msg == nil {
		return nil
	}

	if err := msg.Validate(); err != nil {
		return err
	}

	return nil
}

type noOpKmsgWriter struct{}

func (w *noOpKmsgWriter) Write(_ *KernelMessage) error { return nil }
