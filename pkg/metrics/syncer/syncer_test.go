package syncer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

// mockScraper implements the pkgmetrics.Scraper interface for testing
type mockScraper struct {
	metrics pkgmetrics.Metrics
	err     error
	scrapes int
	mu      sync.Mutex
}

func newMockScraper(metrics pkgmetrics.Metrics, err error) *mockScraper {
	return &mockScraper{
		metrics: metrics,
		err:     err,
	}
}

func (m *mockScraper) Scrape(ctx context.Context) (pkgmetrics.Metrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.scrapes++
	return m.metrics, m.err
}

func (m *mockScraper) getScrapeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.scrapes
}

// mockStore implements the pkgmetrics.Store interface for testing
type mockStore struct {
	records       []pkgmetrics.Metric
	recordErr     error
	purgeCount    int
	purged        int
	purgeErr      error
	readErr       error
	lastPurgeTime time.Time
	mu            sync.Mutex
}

func newMockStore(recordErr, purgeErr, readErr error) *mockStore {
	return &mockStore{
		records:   make([]pkgmetrics.Metric, 0),
		recordErr: recordErr,
		purgeErr:  purgeErr,
		readErr:   readErr,
	}
}

func (m *mockStore) Record(ctx context.Context, ms ...pkgmetrics.Metric) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.recordErr != nil {
		return m.recordErr
	}

	m.records = append(m.records, ms...)
	return nil
}

func (m *mockStore) Read(ctx context.Context, since time.Time) (pkgmetrics.Metrics, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.readErr != nil {
		return nil, m.readErr
	}

	result := make(pkgmetrics.Metrics, 0)
	for _, metric := range m.records {
		if metric.UnixMilliseconds >= since.UnixMilli() {
			result = append(result, metric)
		}
	}

	return result, nil
}

func (m *mockStore) Purge(ctx context.Context, before time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.purgeCount++
	m.lastPurgeTime = before

	if m.purgeErr != nil {
		return 0, m.purgeErr
	}

	// Simulate purging records
	remain := make([]pkgmetrics.Metric, 0)
	purged := 0

	for _, metric := range m.records {
		if metric.UnixMilliseconds >= before.UnixMilli() {
			remain = append(remain, metric)
		} else {
			purged++
		}
	}

	m.records = remain
	m.purged = purged

	return purged, nil
}

func (m *mockStore) getRecordCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.records)
}

func (m *mockStore) getPurgeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.purgeCount
}

func (m *mockStore) getLastPurgeTime() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.lastPurgeTime
}

func TestNewSyncer(t *testing.T) {
	t.Parallel()

	// Set up test dependencies
	scraper := newMockScraper(nil, nil)
	store := newMockStore(nil, nil, nil)

	// Test with valid parameters
	ctx := context.Background()
	syncInterval := 10 * time.Millisecond
	purgeInterval := 50 * time.Millisecond
	retainDuration := 1 * time.Hour

	s := NewSyncer(ctx, scraper, store, syncInterval, purgeInterval, retainDuration)

	require.NotNil(t, s, "syncer should not be nil")
	require.Equal(t, syncInterval, s.scrapeInterval)
	require.Equal(t, purgeInterval, s.purgeInterval)
	require.Equal(t, retainDuration, s.retainDuration)
	require.NotNil(t, s.ctx, "context should not be nil")
	require.NotNil(t, s.cancel, "cancel function should not be nil")
}

func TestSync(t *testing.T) {
	t.Parallel()

	// Create test metrics
	now := time.Now().UnixMilli()
	testMetrics := pkgmetrics.Metrics{
		{
			UnixMilliseconds: now,
			Component:        "test-component",
			Name:             "metric1",
			Value:            42.0,
		},
		{
			UnixMilliseconds: now,
			Component:        "test-component",
			Name:             "metric2",
			Label:            "gpu0",
			Value:            123.45,
		},
	}

	// Test case 1: Successful sync
	t.Run("SuccessfulSync", func(t *testing.T) {
		scraper := newMockScraper(testMetrics, nil)
		store := newMockStore(nil, nil, nil)

		ctx := context.Background()
		s := NewSyncer(ctx, scraper, store, time.Second, time.Second, time.Hour)

		err := s.sync()
		require.NoError(t, err)
		require.Equal(t, 1, scraper.getScrapeCount())
		require.Equal(t, len(testMetrics), store.getRecordCount())
	})

	// Test case 2: Scrape error
	t.Run("ScrapeError", func(t *testing.T) {
		expectedErr := errors.New("scrape error")
		scraper := newMockScraper(nil, expectedErr)
		store := newMockStore(nil, nil, nil)

		ctx := context.Background()
		s := NewSyncer(ctx, scraper, store, time.Second, time.Second, time.Hour)

		err := s.sync()
		require.Error(t, err)
		require.Equal(t, expectedErr, err)
		require.Equal(t, 1, scraper.getScrapeCount())
		require.Equal(t, 0, store.getRecordCount())
	})

	// Test case 3: Store error
	t.Run("StoreError", func(t *testing.T) {
		expectedErr := errors.New("store error")
		scraper := newMockScraper(testMetrics, nil)
		store := newMockStore(expectedErr, nil, nil)

		ctx := context.Background()
		s := NewSyncer(ctx, scraper, store, time.Second, time.Second, time.Hour)

		err := s.sync()
		require.Error(t, err)
		require.Equal(t, expectedErr, err)
		require.Equal(t, 1, scraper.getScrapeCount())
		require.Equal(t, 0, store.getRecordCount())
	})
}

func TestStartStop(t *testing.T) {
	// Test case: Start and Stop
	t.Run("StartStop", func(t *testing.T) {
		scraper := newMockScraper(pkgmetrics.Metrics{}, nil)
		store := newMockStore(nil, nil, nil)

		ctx := context.Background()
		scrapeInterval := 50 * time.Millisecond
		purgeInterval := 100 * time.Millisecond
		retainDuration := 1 * time.Hour

		s := NewSyncer(ctx, scraper, store, scrapeInterval, purgeInterval, retainDuration)

		// Start the syncer
		s.Start()

		// Wait for some time to allow scraping and purging to occur
		time.Sleep(250 * time.Millisecond)

		// Stop the syncer
		s.Stop()

		// Assert that scraping occurred at least once
		require.GreaterOrEqual(t, scraper.getScrapeCount(), 1)

		// Assert that purging occurred at least once
		require.GreaterOrEqual(t, store.getPurgeCount(), 1)

		// Verify that the last purge time is about retainDuration ago
		lastPurgeTime := store.getLastPurgeTime()
		require.NotEqual(t, time.Time{}, lastPurgeTime)

		// Check that the purge was called with a time approximately retainDuration ago
		purgeTimeDiff := time.Until(lastPurgeTime.Add(retainDuration))
		require.InDelta(t, 0, purgeTimeDiff.Seconds(), 1, "Purge time should be approximately retainDuration ago")

		// Wait a bit more and verify counters don't increase after stopping
		currentScrapeCount := scraper.getScrapeCount()
		currentPurgeCount := store.getPurgeCount()

		time.Sleep(200 * time.Millisecond)

		require.Equal(t, currentScrapeCount, scraper.getScrapeCount(), "Scrape count should not increase after stopping")
		require.Equal(t, currentPurgeCount, store.getPurgeCount(), "Purge count should not increase after stopping")
	})

	// Test context cancellation
	t.Run("ContextCancellation", func(t *testing.T) {
		scraper := newMockScraper(pkgmetrics.Metrics{}, nil)
		store := newMockStore(nil, nil, nil)

		ctx, cancel := context.WithCancel(context.Background())
		s := NewSyncer(ctx, scraper, store, 50*time.Millisecond, 100*time.Millisecond, time.Hour)

		// Start the syncer
		s.Start()

		// Wait for some activity
		time.Sleep(150 * time.Millisecond)

		// Get current counts
		currentScrapeCount := scraper.getScrapeCount()
		currentPurgeCount := store.getPurgeCount()

		// Cancel the parent context
		cancel()

		// Wait a bit more
		time.Sleep(200 * time.Millisecond)

		// Verify no more activity after context cancellation
		require.Equal(t, currentScrapeCount, scraper.getScrapeCount(), "Scrape count should not increase after context cancellation")
		require.Equal(t, currentPurgeCount, store.getPurgeCount(), "Purge count should not increase after context cancellation")
	})
}

func TestSyncerWithErrors(t *testing.T) {
	t.Run("ScrapeErrors", func(t *testing.T) {
		scraper := newMockScraper(nil, errors.New("scrape error"))
		store := newMockStore(nil, nil, nil)

		ctx := context.Background()
		s := NewSyncer(ctx, scraper, store, 50*time.Millisecond, 200*time.Millisecond, time.Hour)

		// Start the syncer
		s.Start()

		// Even with errors, the syncer should continue running
		time.Sleep(200 * time.Millisecond)

		// Verify that scrape was attempted multiple times despite errors
		require.GreaterOrEqual(t, scraper.getScrapeCount(), 2)

		// Stop the syncer
		s.Stop()
	})

	t.Run("PurgeErrors", func(t *testing.T) {
		scraper := newMockScraper(pkgmetrics.Metrics{}, nil)
		store := newMockStore(nil, errors.New("purge error"), nil)

		ctx := context.Background()
		s := NewSyncer(ctx, scraper, store, 200*time.Millisecond, 50*time.Millisecond, time.Hour)

		// Start the syncer
		s.Start()

		// Even with errors, the syncer should continue running
		time.Sleep(200 * time.Millisecond)

		// Verify that purge was attempted multiple times despite errors
		require.GreaterOrEqual(t, store.getPurgeCount(), 2)

		// Stop the syncer
		s.Stop()
	})
}
