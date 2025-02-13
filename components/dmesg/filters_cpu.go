package dmesg

import (
	cpu_dmesg "github.com/leptonai/gpud/components/cpu/dmesg"
	cpu_id "github.com/leptonai/gpud/components/cpu/id"
	query_log_common "github.com/leptonai/gpud/pkg/query/log/common"

	"k8s.io/utils/ptr"
)

const (
	// e.g.,
	// INFO: task kcompactd1:1177 blocked for more than 120 seconds.
	// INFO: task jfsmount:136986 blocked for more than 120 seconds.
	// task jfsmount:136986 blocked for more than 120 seconds.
	// "echo 0 > /proc/sys/kernel/hung_task_timeout_secs" disables this message.
	// task:jfsmount        state:D stack:    0 pid: 9831 ppid:  9614 flags:0x00000004
	EventCPUBlockedTooLong = "cpu_blocked_too_long"

	// e.g.,
	// [Sun Jan  5 18:28:55 2025] watchdog: BUG: soft lockup - CPU#18 stuck for 27s! [python3:2254956]
	// [Sun Jan  5 20:25:34 2025] watchdog: BUG: soft lockup - CPU#6 stuck for 48s! [python3:2257218]
	// [Sun Jan  5 18:33:00 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 25s! [pt_data_pin:2273422]
	// [Sun Jan  5 19:42:34 2025] watchdog: BUG: soft lockup - CPU#4 stuck for 23s! [pt_autograd_0:2289563]
	// [Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]
	EventCPUSoftLockup = "cpu_soft_lockup"
)

func DefaultDmesgFiltersForCPU() []*query_log_common.Filter {
	return []*query_log_common.Filter{
		{
			Name:            EventCPUBlockedTooLong,
			Regex:           ptr.To(cpu_dmesg.RegexBlockedTooLong),
			OwnerReferences: []string{cpu_id.Name},
		},
		{
			Name:            EventCPUSoftLockup,
			Regex:           ptr.To(cpu_dmesg.RegexSoftLockup),
			OwnerReferences: []string{cpu_id.Name},
		},
	}
}
