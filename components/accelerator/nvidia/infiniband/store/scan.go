package store

import (
	"context"
	"time"
)

// Scan scans the recent events to mark any events
// (such as "ib port drop").
func (s *ibPortsStore) Scan() error {
	// Read configuration fields under mutex to avoid race conditions
	s.configMu.RLock()
	rootCtx := s.rootCtx
	metadataTable := s.metadataTable
	retentionPeriod := s.retentionPeriod
	s.configMu.RUnlock()

	cctx, cancel := context.WithTimeout(rootCtx, 30*time.Second)
	tombstoneTS, err := getTombstoneTimestamp(cctx, s.dbRO, metadataTable)
	cancel()
	if err != nil {
		return err
	}

	scanSince := s.getTimeNow().Add(-retentionPeriod)
	if !tombstoneTS.IsZero() && tombstoneTS.After(scanSince) {
		scanSince = tombstoneTS
	}

	allDevs := s.getAllDeviceValues()
	allPorts := s.getAllPortValues()

	for dev := range allDevs {
		for port := range allPorts {
			drops, err := s.scanIBPortDrops(dev, port, scanSince)
			if err != nil {
				return err
			}
			for _, rs := range drops {
				if err := s.SetEventType(dev, port, rs.ts, EventTypeIbPortDrop, rs.reason); err != nil {
					return err
				}
			}

			flaps, err := s.scanIBPortFlaps(dev, port, scanSince)
			if err != nil {
				return err
			}
			for _, rs := range flaps {
				if err := s.SetEventType(dev, port, rs.ts, EventTypeIbPortFlap, rs.reason); err != nil {
					return err
				}
			}
		}
	}

	return nil
}
