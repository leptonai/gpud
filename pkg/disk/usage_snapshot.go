package disk

import (
	"sync/atomic"
	"time"
)

// The background disk check normally runs once a minute. Expire its samples
// after five minutes so a stalled collector cannot advertise usage forever.
const nfsUsageMaxAge = 5 * time.Minute

type usageKey struct {
	mountPoint string
	device     string
	fstype     string
}

type usageSample struct {
	at    time.Time
	usage Usage
}

type usageSnapshot struct {
	command string
	usage   map[usageKey]usageSample
}

var latestNFSUsage atomic.Pointer[usageSnapshot]

// PublishNFSUsage replaces the snapshot after a successful background scan.
// Failed scans must not call this: the last successful sample remains usable
// until it expires. Copy values so callers can safely reuse their partitions.
func PublishNFSUsage(command string, parts Partitions) {
	now := time.Now()
	previous := latestNFSUsage.Load()
	snapshot := &usageSnapshot{command: command, usage: make(map[usageKey]usageSample)}
	for _, part := range parts {
		if !DefaultNFSFsTypeFunc(part.Fstype) {
			continue
		}
		key := usageKey{part.MountPoint, part.Device, part.Fstype}
		if part.Mounted && !part.StatTimedOut && part.Usage != nil {
			snapshot.usage[key] = usageSample{at: now, usage: *part.Usage}
		} else if previous != nil && previous.command == command {
			// A failed individual probe must not refresh its previous sample's age.
			if sample, ok := previous.usage[key]; ok && now.Sub(sample.at) < nfsUsageMaxAge {
				snapshot.usage[key] = sample
			}
		}
	}
	latestNFSUsage.Store(snapshot)
}

// CachedNFSUsage returns a copy of a fresh sample without any filesystem I/O.
// Match the collection command and mount identity to avoid using another
// namespace's sample or a different filesystem remounted at the same path.
func CachedNFSUsage(command string, part Partition) *Usage {
	snapshot := latestNFSUsage.Load()
	if snapshot == nil || snapshot.command != command {
		return nil
	}
	sample, ok := snapshot.usage[usageKey{part.MountPoint, part.Device, part.Fstype}]
	if !ok || time.Since(sample.at) >= nfsUsageMaxAge {
		return nil
	}
	return &sample.usage
}
