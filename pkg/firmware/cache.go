package firmware

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

// latest holds the last collection. Machine info builders only read it, so
// login, gossip and the HTTP handler never wait on NVML or sysfs reads.
var latest atomic.Pointer[[]apiv1.MachineFirmware]

// Latest returns a copy of the last collected entries, or nil before the first
// collection or when nothing was found.
func Latest() []apiv1.MachineFirmware {
	p := latest.Load()
	if p == nil || len(*p) == 0 {
		return nil
	}
	rows := slices.Clone(*p)
	for i := range rows {
		rows[i].InstanceNames = slices.Clone(rows[i].InstanceNames)
	}
	return rows
}

// Refresh collects once and replaces the cached entries. The cache is kept
// when ctx is done before the collection finished.
//
// A source or GPU that fails drops its entries until the next refresh. Its
// previous entries are not carried over: they would need their own read time
// inside an entry that groups instances read now.
func Refresh(ctx context.Context, opts Options) {
	rows, err := collect(ctx, opts)
	if err != nil {
		// Cancellation is a normal shutdown; a deadline means the reads were too slow.
		if errors.Is(err, context.Canceled) {
			log.Logger.Debugw("firmware collection canceled", "error", err)
		} else {
			log.Logger.Warnw("firmware collection interrupted", "error", err)
		}
		return
	}
	latest.Store(&rows)
	log.Logger.Debugw("collected firmware versions", "entries", len(rows))
}

// RunPeriodic refreshes every interval until ctx is done. It does not
// collect immediately; call Refresh first for that.
func RunPeriodic(ctx context.Context, opts Options, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		Refresh(ctx, opts)
	}
}

// Start collects in the background now and then every interval until ctx is
// done. The returned channel is closed when the first collection finished.
func Start(ctx context.Context, opts Options, interval time.Duration) <-chan struct{} {
	ready := make(chan struct{})
	go func() {
		Refresh(ctx, opts)
		close(ready)
		RunPeriodic(ctx, opts, interval)
	}()
	return ready
}

// WaitFirst waits for ready to close, at most timeout. It returns false on
// timeout or when ctx is done first. NVML calls cannot be canceled, so the
// wait must be bounded rather than block on a slow GPU.
func WaitFirst(ctx context.Context, ready <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-ready:
		return true
	default:
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-ready:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}
