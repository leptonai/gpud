package events

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// go test -v -run=^$ -bench=BenchmarkRebootsStore_Record_NoExistingEvents -count=1 -timeout=10m -benchmem > /tmp/baseline1.txt
// USE_NEW=true go test -v -run=^$ -bench=BenchmarkRebootsStore_Record_NoExistingEvents -count=1 -timeout=10m -benchmem > /tmp/new1.txt
// benchstat /tmp/baseline1.txt /tmp/new1.txt
func BenchmarkRebootsStore_Record_NoExistingEvents(b *testing.B) {
	store, eventStore, cleanup := setupBenchmarkRebootsStore(b)
	defer cleanup()

	// Populate with mixed events to simulate real-world scenario
	populateMixedEvents(b, eventStore, 100, 500)

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := store.Record(ctx); err != nil {
			b.Fatalf("Record failed: %v", err)
		}
	}
}

// go test -v -run=^$ -bench=BenchmarkRebootsStore_Get_SmallData -count=1 -timeout=10m -benchmem > /tmp/baseline2.txt
// USE_NEW=true go test -v -run=^$ -bench=BenchmarkRebootsStore_Get_SmallData -count=1 -timeout=10m -benchmem > /tmp/new2.txt
// benchstat /tmp/baseline2.txt /tmp/new2.txt
func BenchmarkRebootsStore_Get_SmallData(b *testing.B) {
	store, eventStore, cleanup := setupBenchmarkRebootsStore(b)
	defer cleanup()

	// Populate with mixed events to simulate real-world scenario
	populateMixedEvents(b, eventStore, 1000, 5000)

	ctx := context.Background()
	since := time.Now().UTC().Add(-24 * time.Hour)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := store.Get(ctx, since)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

// go test -v -run=^$ -bench=BenchmarkRebootsStore_Get_LargeData -count=1 -timeout=10m -benchmem > /tmp/baseline3.txt
// USE_NEW=true go test -v -run=^$ -bench=BenchmarkRebootsStore_Get_LargeData -count=1 -timeout=10m -benchmem > /tmp/new3.txt
// benchstat /tmp/baseline3.txt /tmp/new3.txt
func BenchmarkRebootsStore_Get_LargeData(b *testing.B) {
	store, eventStore, cleanup := setupBenchmarkRebootsStore(b)
	defer cleanup()

	// Populate with a large dataset to test SQL query performance
	populateMixedEvents(b, eventStore, 5000, 25000)

	ctx := context.Background()
	since := time.Now().UTC().Add(-7 * 24 * time.Hour)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := store.Get(ctx, since)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

// go test -v -run=^$ -bench=BenchmarkRebootsStore_Record_ExistingEvents -count=1 -timeout=10m -benchmem > /tmp/baseline4.txt
// USE_NEW=true go test -v -run=^$ -bench=BenchmarkRebootsStore_Record_ExistingEvents -count=1 -timeout=10m -benchmem > /tmp/new4.txt
// benchstat /tmp/baseline4.txt /tmp/new4.txt
func BenchmarkRebootsStore_Record_ExistingEvents(b *testing.B) {
	store, eventStore, cleanup := setupBenchmarkRebootsStore(b)
	defer cleanup()

	// Populate with many existing events to test duplicate checking performance
	populateMixedEvents(b, eventStore, 2000, 10000)

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := store.Record(ctx); err != nil {
			b.Fatalf("Record failed: %v", err)
		}
	}
}

func setupBenchmarkRebootsStore(b *testing.B) (RebootsStore, eventstore.Store, func()) {
	// Create temporary database file for benchmark
	tmpf, err := os.CreateTemp(os.TempDir(), "benchmark-sqlite")
	if err != nil {
		b.Fatalf("failed to create temp file: %v", err)
	}

	dbRW, err := sqlite.Open(tmpf.Name())
	if err != nil {
		b.Fatalf("failed to open database: %v", err)
	}

	dbRO, err := sqlite.Open(tmpf.Name(), sqlite.WithReadOnly(true))
	if err != nil {
		b.Fatalf("failed to open database: %v", err)
	}

	database, err := eventstore.New(dbRW, dbRO, 0)
	if err != nil {
		b.Fatalf("failed to create eventstore: %v", err)
	}

	// Create a mock getLastRebootTime function for consistent benchmarking
	mockGetLastRebootTime := func(ctx context.Context) (time.Time, error) {
		return time.Now().UTC(), nil
	}

	bucket, err := database.Bucket(RebootBucketName, eventstore.WithDisablePurge())
	if err != nil {
		b.Fatalf("failed to create bucket: %v", err)
	}
	cleanup := func() {
		bucket.Close()
		_ = dbRW.Close()
		_ = dbRO.Close()
		_ = os.Remove(tmpf.Name())
	}

	store := &rebootsStore{
		getTimeNowFunc:    func() time.Time { return time.Now().UTC() },
		getLastRebootTime: mockGetLastRebootTime,
		bucket:            bucket,
	}

	return store, database, cleanup
}

func populateMixedEvents(b *testing.B, eventStore eventstore.Store, numRebootEvents, numOtherEvents int) {
	ctx := context.Background()
	bucket, err := eventStore.Bucket(RebootBucketName, eventstore.WithDisablePurge())
	if err != nil {
		b.Fatalf("failed to create bucket: %v", err)
	}
	defer bucket.Close()

	now := time.Now().UTC()

	// Add reboot events
	for i := 0; i < numRebootEvents; i++ {
		ev := eventstore.Event{
			Time:    now.Add(time.Duration(i) * time.Hour),
			Name:    RebootEventName,
			Type:    string(apiv1.EventTypeWarning),
			Message: fmt.Sprintf("system reboot detected %d", i),
		}
		if err := bucket.Insert(ctx, ev); err != nil {
			b.Fatalf("failed to insert reboot event: %v", err)
		}
	}

	// Add other events (non-reboot events in the same bucket)
	for i := 0; i < numOtherEvents; i++ {
		ev := eventstore.Event{
			Time:    now.Add(time.Duration(i) * time.Hour),
			Name:    fmt.Sprintf("kmsg-%d", i%3),
			Type:    string(apiv1.EventTypeInfo),
			Message: fmt.Sprintf("kmsg event %d", i),
		}
		if err := bucket.Insert(ctx, ev); err != nil {
			b.Fatalf("failed to insert other event: %v", err)
		}
	}
}
