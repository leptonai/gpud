//nolint:revive // package name follows the directory import path used across the codebase.
package os

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/leptonai/gpud/pkg/log"
)

const (
	// DefaultBlockedProcessPersistenceThreshold is the number of consecutive
	// checks a process must remain blocked (Linux D-state) before it is
	// considered persistent. The OS component checks once per minute: five
	// consecutive checks means the process has been blocked for at least four
	// minutes of wall time (persistenceThreshold check intervals).
	//
	// Why five: transient D-state is normal -- most blocking disk I/O
	// (reads, writes, page faults hitting storage) passes through D for
	// milliseconds to seconds -- so a single observation must never flag a
	// node. The 2026-08-12 validation found zero D-state processes on healthy
	// production GPU nodes (instant scans plus a 30x10s sampling window),
	// while an induced real D-state process (dd on a suspended device-mapper
	// device) persisted >=6 minutes. Five one-minute checks sit between the
	// two: long enough to ignore ordinary I/O waits, short enough to surface
	// a wedged kernel path within minutes after it stalls.
	DefaultBlockedProcessPersistenceThreshold = 5

	// defaultBlockedProcessNameRegex matches NVIDIA management processes
	// (e.g., nvidia-smi, nvidia-persistenced) per LEP-6029.
	defaultBlockedProcessNameRegex = "^nvidia"
)

// DefaultBlockedProcessNameRegexes returns the default process-name regexes
// used to escalate persistent D-state processes on machines with NVIDIA GPUs.
func DefaultBlockedProcessNameRegexes() []string {
	return []string{defaultBlockedProcessNameRegex}
}

// BlockedProcessThresholds configures the D-state (blocked) process tracking
// of the os component (LEP-6029), following the same default-override pattern
// as components/accelerator/nvidia/gpu-counts/threshold.go.
type BlockedProcessThresholds struct {
	// PersistenceThreshold is the number of consecutive checks a process must
	// remain blocked before it is considered persistent. Values <= 0 reset to
	// DefaultBlockedProcessPersistenceThreshold.
	PersistenceThreshold int `json:"persistence_threshold"`

	// NameRegexes gates which persistent D-state processes escalate to
	// unhealthy with a repair suggestion. An empty set disables D-state
	// process checking entirely.
	//
	// The gate exists because D-state alone does not identify the cause: a
	// dd stuck on a suspended device and an nvidia-smi stuck in a wedged
	// driver show the same D state letter in /proc/<pid>/stat (both become
	// process.Blocked at the level gpud reads). A persistent D-state process from
	// any name degrades the component (something in the kernel is stuck and
	// SIGKILL cannot clear it), but a reboot suggestion is only attached when
	// the name matches -- i.e., when the stuck process implicates the GPU
	// driver path, as in the LEP-6029 incident (nvidia-smi in D during a
	// kubelet package upgrade).
	NameRegexes []string `json:"name_regexes"`

	// compiledNameRegexes is the compiled form of NameRegexes, populated by
	// newBlockedProcessThresholds (and thus SetDefaultBlockedProcessThresholds).
	compiledNameRegexes []*regexp.Regexp
}

// IsZero returns true when no process-name regexes are configured, meaning
// D-state process checking is disabled.
func (t BlockedProcessThresholds) IsZero() bool {
	return len(t.NameRegexes) == 0
}

// MatchesName returns true if the process name matches any configured regex.
func (t BlockedProcessThresholds) MatchesName(name string) bool {
	for _, m := range t.compiledNameRegexes {
		if m.MatchString(name) {
			return true
		}
	}
	return false
}

var (
	defaultBlockedProcessThresholdsMu sync.RWMutex
	// defaultBlockedProcessThresholds is the built-in default: D-state checking
	// stays disabled (empty NameRegexes) unless auto-enabled on machines with
	// NVIDIA GPUs or configured via flags (see ApplyOSBlockedProcessThresholds
	// in cmd/gpud/common), so upgrading gpud never changes D-state alerting on
	// non-GPU machines.
	defaultBlockedProcessThresholds = BlockedProcessThresholds{
		PersistenceThreshold: DefaultBlockedProcessPersistenceThreshold,
	}

	startupBlockedProcessThresholdsMu sync.RWMutex
	// startupBlockedProcessThresholds records the thresholds resolved at
	// process startup (CLI flags + NVIDIA GPU auto-detection, recorded by
	// ApplyOSBlockedProcessThresholds in cmd/gpud/common). The session
	// updateConfig fallback restores this baseline when the control-plane
	// config does not specify os thresholds, so unrelated config pushes do
	// not silently disable D-state tracking on GPU machines. Nil until
	// recorded.
	startupBlockedProcessThresholds *BlockedProcessThresholds
)

// GetDefaultBlockedProcessThresholds returns the configured default thresholds.
func GetDefaultBlockedProcessThresholds() BlockedProcessThresholds {
	defaultBlockedProcessThresholdsMu.RLock()
	defer defaultBlockedProcessThresholdsMu.RUnlock()
	ret := defaultBlockedProcessThresholds
	ret.NameRegexes = append([]string(nil), ret.NameRegexes...)
	return ret
}

// SetDefaultBlockedProcessThresholds validates, compiles, and updates the
// default thresholds.
func SetDefaultBlockedProcessThresholds(th BlockedProcessThresholds) error {
	normalized, err := newBlockedProcessThresholds(th.PersistenceThreshold, th.NameRegexes)
	if err != nil {
		return err
	}

	defaultBlockedProcessThresholdsMu.Lock()
	defer defaultBlockedProcessThresholdsMu.Unlock()
	defaultBlockedProcessThresholds = normalized

	log.Logger.Infow("set blocked process thresholds",
		"persistenceThreshold", normalized.PersistenceThreshold,
		"nameRegexes", normalized.NameRegexes,
	)
	return nil
}

// SetStartupBlockedProcessThresholds records the startup-resolved thresholds
// as the baseline that session updateConfig falls back to when the
// control-plane config omits the os component. Called once from the gpud
// run/scan command wiring after flag resolution.
func SetStartupBlockedProcessThresholds(th BlockedProcessThresholds) {
	startupBlockedProcessThresholdsMu.Lock()
	defer startupBlockedProcessThresholdsMu.Unlock()
	copied := th
	copied.NameRegexes = append([]string(nil), th.NameRegexes...)
	startupBlockedProcessThresholds = &copied
}

// GetStartupBlockedProcessThresholds returns the thresholds resolved at
// process startup. If none were recorded (e.g., tests), it returns the
// current defaults so the updateConfig fallback leaves them unchanged.
func GetStartupBlockedProcessThresholds() BlockedProcessThresholds {
	startupBlockedProcessThresholdsMu.RLock()
	recorded := startupBlockedProcessThresholds
	startupBlockedProcessThresholdsMu.RUnlock()
	if recorded == nil {
		return GetDefaultBlockedProcessThresholds()
	}
	ret := *recorded
	ret.NameRegexes = append([]string(nil), ret.NameRegexes...)
	return ret
}

// newBlockedProcessThresholds validates and compiles the regexes, trimming
// blanks and normalizing a non-positive persistence threshold to the default.
func newBlockedProcessThresholds(persistenceThreshold int, nameRegexes []string) (BlockedProcessThresholds, error) {
	compiled := make([]*regexp.Regexp, 0, len(nameRegexes))
	cleaned := make([]string, 0, len(nameRegexes))
	for _, r := range nameRegexes {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		c, err := regexp.Compile(r)
		if err != nil {
			return BlockedProcessThresholds{}, fmt.Errorf("invalid blocked process name regex %q: %w", r, err)
		}
		compiled = append(compiled, c)
		cleaned = append(cleaned, r)
	}

	if persistenceThreshold <= 0 {
		persistenceThreshold = DefaultBlockedProcessPersistenceThreshold
	}

	return BlockedProcessThresholds{
		PersistenceThreshold: persistenceThreshold,
		NameRegexes:          cleaned,
		compiledNameRegexes:  compiled,
	}, nil
}
