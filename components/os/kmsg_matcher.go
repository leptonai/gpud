package os

import (
	"fmt"
	"regexp"
	"strconv"
)

const (
	// e.g.,
	// [...] VFS: file-max limit 1000000 reached
	// [...] VFS: file-max limit <number> reached
	//
	// ref.
	// https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
	eventNameVFSFileMaxLimitReached = "vfs_file_max_limit_reached"
	regexVFSFileMaxLimitReached     = `VFS: file-max limit \d+ reached`
	messageVFSFileMaxLimitReached   = "VFS file-max limit reached"

	// Kernel panic event constants
	// if "/proc/sys/kernel/panic" is set >0, the system auto-reboots
	// thus no need to set suggested action or health state
	// ref. https://cloud.google.com/compute/docs/troubleshooting/kernel-panic
	eventNameKernelPanic = "kernel_panic"
)

var (
	compiledVFSFileMaxLimitReached = regexp.MustCompile(regexVFSFileMaxLimitReached)

	// Kernel panic regex patterns
	// ref. https://cloud.google.com/compute/docs/troubleshooting/kernel-panic
	kernelPanicStartRegexps = []*regexp.Regexp{
		// Match "Kernel panic" or "Kernel Panic" (case insensitive for 'P')
		regexp.MustCompile(`Kernel [Pp]anic`),
	}

	// e.g., CPU: 24 PID: 1364 Comm: khungtaskd Tainted: P           OE     5.15.0-1053-nvidia #54-Ubuntu
	kernelPanicCPUPIDRegexp = regexp.MustCompile(`CPU: (\d+) PID: (\d+) Comm: (\S+)`)
)

// Returns true if the line indicates that the file-max limit has been reached.
// ref. https://docs.kernel.org/admin-guide/sysctl/fs.html#file-max-file-nr
func HasVFSFileMaxLimitReached(line string) bool {
	if match := compiledVFSFileMaxLimitReached.FindStringSubmatch(line); match != nil {
		return true
	}
	return false
}

// KernelPanicInstance contains information related to a kernel panic event
type KernelPanicInstance struct {
	// Process ID from the CPU line
	PID int
	// CPU number where the panic occurred
	CPU int
	// Process name (e.g., "khungtaskd")
	ProcessName string
}

func (k *KernelPanicInstance) Summary() string {
	if k == nil {
		return ""
	}
	return fmt.Sprintf("Kernel panic detected - CPU: %d, PID: %d, Process: %s", k.CPU, k.PID, k.ProcessName)
}

// createKernelPanicMatchFunc creates a stateful match function for kernel panics
func createKernelPanicMatchFunc() func(line string) (eventName string, message string) {
	// Track state across multiple lines
	readingPanicMessages := false
	var panicCurrentInstance *KernelPanicInstance
	linesReadSincePanicStart := 0
	const maxLinesAfterPanicStart = 10 // Look for CPU/PID info within 10 lines

	return func(line string) (eventName string, message string) {
		// Check if this is the start of a new kernel panic
		isNewPanicStart := checkIfStartOfPanicMessages(line)

		if isNewPanicStart {
			// If we were already reading panic messages but found a new panic,
			// return the previous panic event without CPU/PID info
			if readingPanicMessages {
				eventName = eventNameKernelPanic
				message = "Kernel panic detected (no CPU/PID info found)"
			}

			// Start tracking a new panic event
			readingPanicMessages = true
			panicCurrentInstance = &KernelPanicInstance{}
			linesReadSincePanicStart = 0
			return eventName, message
		}

		// If not reading panic messages, ignore this line
		if !readingPanicMessages {
			return "", ""
		}

		linesReadSincePanicStart++

		// Try to extract CPU and PID information
		cpuPIDFound := extractCPUandPID(line, panicCurrentInstance)
		if cpuPIDFound && panicCurrentInstance.PID >= 0 {
			// We have the info we need - panic event is complete
			eventName = eventNameKernelPanic
			message = panicCurrentInstance.Summary()

			// Reset state
			readingPanicMessages = false
			panicCurrentInstance = nil
			linesReadSincePanicStart = 0
			return eventName, message
		}

		// If we've read too many lines without finding CPU/PID, return the panic event anyway
		if linesReadSincePanicStart >= maxLinesAfterPanicStart {
			eventName = eventNameKernelPanic
			message = "Kernel panic detected (no CPU/PID info found)"

			// Reset state
			readingPanicMessages = false
			panicCurrentInstance = nil
			linesReadSincePanicStart = 0
			return eventName, message
		}

		// Still collecting information
		return "", ""
	}
}

// Helper functions for kernel panic detection
func checkIfStartOfPanicMessages(line string) bool {
	for _, regex := range kernelPanicStartRegexps {
		if regex.MatchString(line) {
			return true
		}
	}
	return false
}

func extractCPUandPID(line string, instance *KernelPanicInstance) bool {
	matches := kernelPanicCPUPIDRegexp.FindStringSubmatch(line)
	if len(matches) < 4 {
		return false
	}

	cpu, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}
	instance.CPU = cpu

	pid, err := strconv.Atoi(matches[2])
	if err != nil {
		return false
	}
	instance.PID = pid

	instance.ProcessName = matches[3]

	return true
}

// Global kernel panic matcher instance
var kernelPanicMatcher = createKernelPanicMatchFunc()

func Match(line string) (eventName string, message string) {
	// First check kernel panic matcher (stateful)
	if eventName, message := kernelPanicMatcher(line); eventName != "" {
		return eventName, message
	}

	// Then check other matchers
	for _, m := range getMatches() {
		if m.check(line) {
			return m.eventName, m.message
		}
	}
	return "", ""
}

type match struct {
	check     func(string) bool
	eventName string
	regex     string
	message   string
}

func getMatches() []match {
	return []match{
		{check: HasVFSFileMaxLimitReached, eventName: eventNameVFSFileMaxLimitReached, regex: regexVFSFileMaxLimitReached, message: messageVFSFileMaxLimitReached},
	}
}
