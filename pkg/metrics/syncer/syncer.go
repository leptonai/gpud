// Package syncer provides a syncer for the metrics.
package syncer

import (
	"context"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

type Syncer struct {
	ctx            context.Context
	cancel         context.CancelFunc
	scraper        pkgmetrics.Scraper
	store          pkgmetrics.Store
	scrapeInterval time.Duration
	purgeInterval  time.Duration
	retainDuration time.Duration
}

func NewSyncer(ctx context.Context, scraper pkgmetrics.Scraper, store pkgmetrics.Store, scrapeInterval time.Duration, purgeInterval time.Duration, retainDuration time.Duration) *Syncer {
	cctx, cancel := context.WithCancel(ctx)
	s := &Syncer{
		ctx:            cctx,
		cancel:         cancel,
		scraper:        scraper,
		store:          store,
		scrapeInterval: scrapeInterval,
		purgeInterval:  purgeInterval,
		retainDuration: retainDuration,
	}
	return s
}

func (s *Syncer) Start() {
	go func() {
		ticker := time.NewTicker(s.scrapeInterval)
		defer ticker.Stop()

		log.Logger.Infow("start scrap and sync metrics")
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}

			if err := s.sync(); err != nil {
				log.Logger.Errorw("failed to sync metrics", "error", err)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(s.purgeInterval)
		defer ticker.Stop()

		log.Logger.Infow("start purging metrics")
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}

			before := time.Now().UTC().Add(-s.retainDuration)
			if purged, err := s.store.Purge(s.ctx, before); err != nil {
				log.Logger.Errorw("failed to purge metrics", "error", err)
			} else {
				log.Logger.Infow("purged metrics", "purged", purged)
			}
		}
	}()
}

func (s *Syncer) sync() error {
	ms, err := s.scraper.Scrape(s.ctx)
	if err != nil {
		return err
	}
	return s.store.Record(s.ctx, ms...)
}

func (s *Syncer) Stop() {
	log.Logger.Infow("stopping syncer")

	s.cancel()
}
