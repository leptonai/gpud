package dmesg

import "regexp"

const (
	// e.g.,
	// INFO: task kcompactd1:1177 blocked for more than 120 seconds.
	// INFO: task jfsmount:136986 blocked for more than 120 seconds.
	// task jfsmount:136986 blocked for more than 120 seconds.
	// "echo 0 > /proc/sys/kernel/hung_task_timeout_secs" disables this message.
	// task:jfsmount        state:D stack:    0 pid: 9831 ppid:  9614 flags:0x00000004
	RegexBlockedTooLong = `(?:INFO: )?task ([^:]+):.+blocked for more than 120 seconds`

	// e.g.,
	// [Sun Jan  5 18:28:55 2025] watchdog: BUG: soft lockup - CPU#18 stuck for 27s! [python3:2254956]
	// [Sun Jan  5 20:25:34 2025] watchdog: BUG: soft lockup - CPU#6 stuck for 48s! [python3:2257218]
	// [Sun Jan  5 18:33:00 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 25s! [pt_data_pin:2273422]
	// [Sun Jan  5 19:42:34 2025] watchdog: BUG: soft lockup - CPU#4 stuck for 23s! [pt_autograd_0:2289563]
	// [Sun Jan  5 18:37:06 2025] watchdog: BUG: soft lockup - CPU#0 stuck for 27s! [cuda-EvtHandlr:2255424]
	RegexSoftLockup = `soft lockup - CPU#\d+ stuck for \d+s! \[([^:]+):`
)

var (
	compiledBlockedTooLong = regexp.MustCompile(RegexBlockedTooLong)
	compiledSoftLockup     = regexp.MustCompile(RegexSoftLockup)
)

// Returns the task name and true if the line indicates that a task is hung too long.
func HasBlockedTooLong(line string) (string, bool) {
	if match := compiledBlockedTooLong.FindStringSubmatch(line); match != nil {
		return match[1], true
	}
	return "", false
}

// Returns the task name and true if the line indicates a CPU soft lockup.
func HasSoftLockup(line string) (string, bool) {
	if match := compiledSoftLockup.FindStringSubmatch(line); match != nil {
		return match[1], true
	}
	return "", false
}
