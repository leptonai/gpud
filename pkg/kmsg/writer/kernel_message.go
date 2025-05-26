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

type KernelMessagePriority string

const (
	KernelMessagePriorityEmerg   KernelMessagePriority = "KERN_EMERG"
	KernelMessagePriorityAlert   KernelMessagePriority = "KERN_ALERT"
	KernelMessagePriorityCrit    KernelMessagePriority = "KERN_CRIT"
	KernelMessagePriorityError   KernelMessagePriority = "KERN_ERR"
	KernelMessagePriorityWarning KernelMessagePriority = "KERN_WARNING"
	KernelMessagePriorityNotice  KernelMessagePriority = "KERN_NOTICE"
	KernelMessagePriorityInfo    KernelMessagePriority = "KERN_INFO"
	KernelMessagePriorityDebug   KernelMessagePriority = "KERN_DEBUG"
	KernelMessagePriorityDefault KernelMessagePriority = "KERN_DEFAULT"
)

// KernelMessage represents a kernel message.
type KernelMessage struct {
	// Priority is the priority of the kernel message.
	// ref. https://github.com/torvalds/linux/blob/master/tools/include/linux/kern_levels.h#L8-L15
	Priority KernelMessagePriority `json:"priority"`
	// Message is the message of the kernel message.
	Message string `json:"message"`
}

// Validate validates the kernel message.
func (m *KernelMessage) Validate() error {
	if len(m.Message) > MaxPrintkRecordLength {
		return fmt.Errorf("message length exceeds the maximum length of %d", MaxPrintkRecordLength)
	}
	m.Priority = ConvertKernelMessagePriority(string(m.Priority))
	return nil
}

// ref. https://github.com/torvalds/linux/blob/master/tools/include/linux/kern_levels.h#L8-L15
func ConvertKernelMessagePriority(s string) KernelMessagePriority {
	switch s {
	case "KERN_EMERG", "kern.emerg":
		return KernelMessagePriorityEmerg
	case "KERN_ALERT", "kern.alert":
		return KernelMessagePriorityAlert
	case "KERN_CRIT", "kern.crit":
		return KernelMessagePriorityCrit
	case "KERN_ERR", "kern.err":
		return KernelMessagePriorityError
	case "KERN_WARNING", "kern.warning", "kern.warn":
		return KernelMessagePriorityWarning
	case "KERN_NOTICE", "kern.notice":
		return KernelMessagePriorityNotice
	case "KERN_INFO", "kern.info":
		return KernelMessagePriorityInfo
	case "KERN_DEBUG", "kern.debug":
		return KernelMessagePriorityDebug
	case "KERN_DEFAULT", "kern.default":
		return KernelMessagePriorityDefault
	default: // unknown priority, default to KERN_INFO
		return KernelMessagePriorityInfo
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
