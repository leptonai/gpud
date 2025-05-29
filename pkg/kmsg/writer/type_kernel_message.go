package writer

import (
	"fmt"
	"log/syslog"
)

// MaxPrintkRecordLength is the maximum length of a printk record.
// ref.
// "PRINTKRB_RECORD_MAX" in https://github.com/torvalds/linux/blob/94305e83eccb3120c921cd3a015cd74731140bac/kernel/printk/internal.h#L52
// "PRINTK_PREFIX_MAX" in https://github.com/torvalds/linux/blob/94305e83eccb3120c921cd3a015cd74731140bac/kernel/printk/internal.h#L40C9-L40C26
const MaxPrintkRecordLength = 1024 - 48

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

// SyslogPriority converts KernelMessagePriority to syslog priority value.
// Default facility is LOG_USER, severity mapping follows standard syslog levels.
// ref. https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
func (priority KernelMessagePriority) SyslogPriority() int {
	normalizedPriority := ConvertKernelMessagePriority(string(priority))

	// Map kernel message priorities to syslog severity levels
	var severity int
	switch normalizedPriority {
	case KernelMessagePriorityEmerg:
		severity = 0 // Emergency: system is unusable
	case KernelMessagePriorityAlert:
		severity = 1 // Alert: action must be taken immediately
	case KernelMessagePriorityCrit:
		severity = 2 // Critical: critical conditions
	case KernelMessagePriorityError:
		severity = 3 // Error: error conditions
	case KernelMessagePriorityWarning:
		severity = 4 // Warning: warning conditions
	case KernelMessagePriorityNotice:
		severity = 5 // Notice: normal but significant condition
	case KernelMessagePriorityInfo:
		severity = 6 // Informational: informational messages
	case KernelMessagePriorityDebug:
		severity = 7 // Debug: debug-level messages
	default:
		severity = 6 // Default to INFO
	}

	// "default kernel log priority and the facility number is set to DEFAULT_LOG_USER (1)"
	// "not possible to inject messages from userspace with the facility number LOG_KERN (0),
	// "to make sure that the origin of the messages can always be reliably determined."
	// ref. https://www.kernel.org/doc/Documentation/ABI/testing/dev-kmsg
	//
	// See https://github.com/leptonai/gpud/pull/864 on how to inject messages
	// using a kernel module.
	return int(syslog.LOG_SYSLOG) + severity
}
