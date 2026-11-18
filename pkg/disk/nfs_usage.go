package disk

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

const maxNFSUsageWorkers = 32

type nfsUsageWork struct {
	done  chan struct{}
	usage *Usage
	err   error
}

// NFSUsageCollector bounds the caller's wait without pretending a context can
// interrupt statfs. Slots remain occupied until the underlying I/O finishes.
type NFSUsageCollector struct {
	mu      sync.Mutex
	pending map[string]*nfsUsageWork
	slots   chan struct{}
	probe   func(context.Context, Partition, string) (*Usage, error)
}

// Collect uses metadata-only inventory supplied by the caller. Slow mounts do
// not suppress successful samples from other mounts or stall health publication.
func (c *NFSUsageCollector) Collect(ctx context.Context, mounts Partitions, command string) Partitions {
	parts := make(Partitions, 0, len(mounts))
	seen := make(map[string]int)
	for _, mount := range mounts {
		if i, ok := seen[mount.MountPoint]; ok {
			parts[i] = mount
		} else {
			seen[mount.MountPoint] = len(parts)
			parts = append(parts, mount)
		}
	}
	work := make([]*nfsUsageWork, len(parts))
	c.mu.Lock()
	if c.pending == nil {
		c.pending = make(map[string]*nfsUsageWork)
		c.slots = make(chan struct{}, maxNFSUsageWorkers)
	}
	// Never reuse a late result. Remounts and retries require fresh evidence.
	for key, w := range c.pending {
		select {
		case <-w.done:
			delete(c.pending, key)
		default:
		}
	}
	for i := range parts {
		p := &parts[i]
		p.Mounted, p.Usage, p.StatTimedOut, p.StatError = false, nil, false, ""
		key := command + "\x00" + p.MountPoint
		// Key by path rather than device: replacing a mount must not bypass a
		// blocked worker. The global cap also bounds churn across mount paths.
		if c.pending[key] != nil || ctx.Err() != nil {
			p.StatTimedOut = true
			continue
		}
		w := &nfsUsageWork{done: make(chan struct{})}
		c.pending[key], work[i] = w, w
		probe := c.probe
		if probe == nil {
			probe = probeNFSUsage
		}
		go func(part Partition) {
			defer close(w.done)
			select {
			case c.slots <- struct{}{}:
				defer func() { <-c.slots }()
			case <-ctx.Done():
				w.err = ctx.Err()
				return
			}
			if ctx.Err() != nil {
				w.err = ctx.Err()
				return
			}
			w.usage, w.err = probe(ctx, part, command)
			if ctx.Err() != nil {
				w.usage, w.err = nil, ctx.Err()
			}
		}(*p)
	}
	c.mu.Unlock()
	for i, w := range work {
		if w == nil {
			continue
		}
		select {
		case <-w.done:
		default:
			select {
			case <-w.done:
			case <-ctx.Done():
				// A fast mount may have finished while we waited for a slow one.
				select {
				case <-w.done:
				default:
					parts[i].StatTimedOut = true
					continue
				}
			}
		}
		if w.err != nil {
			parts[i].StatTimedOut = errors.Is(w.err, context.DeadlineExceeded) || errors.Is(w.err, context.Canceled)
			if !parts[i].StatTimedOut {
				parts[i].StatError = w.err.Error()
			}
		} else if w.usage == nil {
			parts[i].StatError = "filesystem usage unavailable"
		} else {
			u := *w.usage
			parts[i].Mounted, parts[i].Usage = true, &u
		}
	}
	return parts
}

func probeNFSUsage(ctx context.Context, part Partition, command string) (*Usage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if command != "" {
		output, err := runDfCommand(ctx, command, "-- "+quoteShellArgument(part.MountPoint))
		if err != nil {
			return nil, err
		}
		parts, err := parseDfOutput(output)
		if err != nil {
			return nil, err
		}
		// df can fall back to the underlying local filesystem after an unmount.
		if len(parts) != 1 || parts[0].MountPoint != part.MountPoint || parts[0].Device != part.Device || parts[0].Fstype != part.Fstype || parts[0].Usage == nil {
			return nil, fmt.Errorf("filesystem identity changed while collecting %q", part.MountPoint)
		}
		return parts[0].Usage, nil
	}
	return probeNativeNFSUsage(ctx, part)
}
