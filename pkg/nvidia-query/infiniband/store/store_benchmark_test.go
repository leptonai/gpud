package store

import (
	"context"
	"database/sql"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/assert"

	_ "github.com/mattn/go-sqlite3"

	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
	"github.com/leptonai/gpud/pkg/sqlite"
)

// BENCHMARK=true go test -v -run=TestSimulatedEvents -timeout=10m
//
// ingest 7 days, every 30 seconds, 8 ib ports == 20 MB
// purge to retain the last 3 days + compact == 8 MB
// thus the default durations are safe to use
func TestSimulatedEvents(t *testing.T) {
	if os.Getenv("BENCHMARK") != "true" {
		t.Skip("skipping benchmark test")
	}

	t.Parallel()

	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create infiniband store
	store, err := New(ctx, dbRW, dbRO)
	assert.NoError(t, err)

	// data ingestion, every 30 seconds
	// 7 days of data retention
	daysToIngest := 7
	intervalSeconds := 30
	eventsN := daysToIngest * 24 * 60 * 60 / intervalSeconds

	// Generate 8 IB ports as requested - realistic H100/A100 cluster setup
	deviceNames := []string{"mlx5_0", "mlx5_1", "mlx5_2", "mlx5_3", "mlx5_4", "mlx5_5", "mlx5_6", "mlx5_7"}

	// Define realistic port profiles based on actual cluster behavior
	portProfiles := make([]portProfile, len(deviceNames))

	for i := range portProfiles {
		// 80% healthy ports, 20% problematic - typical for production clusters
		if rand.Float32() < 0.8 {
			// Healthy port profile
			portProfiles[i] = portProfile{
				isHealthy:         true,
				baseState:         "Active",
				basePhysicalState: "LinkUp",
				baseRate:          400,                  // High-performance
				baseLinkDownCount: uint64(rand.Intn(5)), // Low link down count
				failureRate:       0.001,                // Very low failure rate
				recoveryRate:      0.9,                  // High recovery rate
			}
		} else {
			// Problematic port profile
			problemType := rand.Float32()
			if problemType < 0.5 {
				// Intermittent connection issues
				portProfiles[i] = portProfile{
					isHealthy:         false,
					baseState:         "Active",
					basePhysicalState: "LinkUp",
					baseRate:          200,                        // Degraded performance
					baseLinkDownCount: uint64(rand.Intn(50) + 10), // Higher link down count
					failureRate:       0.05,                       // Higher failure rate
					recoveryRate:      0.7,                        // Lower recovery rate
				}
			} else {
				// Persistently down port
				portProfiles[i] = portProfile{
					isHealthy:         false,
					baseState:         "Down",
					basePhysicalState: "Disabled",
					baseRate:          100,                         // Lowest rate when working
					baseLinkDownCount: uint64(rand.Intn(100) + 20), // High link down count
					failureRate:       0.1,                         // High failure rate
					recoveryRate:      0.3,                         // Low recovery rate
				}
			}
		}
	}

	now := time.Now()
	t.Logf("Starting realistic cluster simulation: %d events over %d days", eventsN, daysToIngest)
	t.Logf("Port profiles: %d healthy, %d problematic ports",
		countHealthyPorts(portProfiles), len(portProfiles)-countHealthyPorts(portProfiles))

	// Track port states for realistic state transitions
	currentPortStates := make([]portProfile, len(deviceNames))
	copy(currentPortStates, portProfiles)

	for i := 0; i < eventsN; i++ {
		eventTime := now.Add(time.Duration(i*intervalSeconds) * time.Second)

		// Create realistic IB ports with state evolution
		ibPorts := make([]infiniband.IBPort, len(deviceNames))
		for j := 0; j < len(deviceNames); j++ {
			// Simulate realistic state transitions based on port profile
			profile := &currentPortStates[j]

			// Determine current state based on profile and random events
			currentState := profile.baseState
			currentPhysicalState := profile.basePhysicalState
			currentRate := profile.baseRate

			// Simulate failures and recoveries
			if profile.isHealthy && rand.Float32() < profile.failureRate {
				// Healthy port experiences temporary issue
				currentState = "Init"
				currentPhysicalState = "Polling"
				currentRate = 100 // Degraded performance during issues
			} else if !profile.isHealthy && rand.Float32() < profile.recoveryRate {
				// Problematic port temporarily recovers
				currentState = "Active"
				currentPhysicalState = "LinkUp"
			}

			// Simulate link down events realistically
			linkDownIncrement := uint64(0)
			if currentState == "Down" || currentPhysicalState == "Disabled" {
				// Increment link down counter occasionally for failed ports
				if rand.Float32() < 0.1 {
					linkDownIncrement = 1
				}
			} else if currentState == "Init" || currentPhysicalState == "Polling" {
				// Higher chance of link down increment during transitions
				if rand.Float32() < 0.3 {
					linkDownIncrement = 1
				}
			}

			// Add some ports with multiple port numbers for realism
			portNumber := uint(1)
			if j < 2 && rand.Float32() < 0.3 {
				portNumber = uint(2) // Some devices have dual ports
			}

			ibPorts[j] = infiniband.IBPort{
				Device:          deviceNames[j],
				Port:            portNumber,
				State:           currentState,
				PhysicalState:   currentPhysicalState,
				RateGBSec:       currentRate,
				LinkLayer:       "Infiniband", // Only Infiniband ports are stored
				TotalLinkDowned: profile.baseLinkDownCount + linkDownIncrement,
			}

			// Update running link down count
			profile.baseLinkDownCount += linkDownIncrement
		}

		// Insert the IB ports into the store
		if err := store.Insert(eventTime, ibPorts); err != nil {
			t.Fatalf("failed to insert IB ports at event %d: %v", i, err)
		}

		// Log progress every 1000 events
		if i%1000 == 0 {
			t.Logf("Progress: %d/%d events ingested", i, eventsN)
		}
	}

	t.Logf("Ingested %d events with %d IB ports each", eventsN, len(deviceNames))

	// Measure database size before compaction
	size, err := sqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read db size: %v", err)
	}
	t.Logf("DB size before compaction: %s", humanize.Bytes(size))

	// Compact the database
	if err := sqlite.Compact(ctx, dbRW); err != nil {
		t.Fatalf("failed to compact db: %v", err)
	}

	// Measure database size after compaction
	size, err = sqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read db size: %v", err)
	}
	t.Logf("DB size after compaction: %s", humanize.Bytes(size))

	// Simulate purging old data (purge data older than 3 days)
	t.Logf("Starting purge operation...")
	purgeBeforeTime := now.Add(time.Duration(daysToIngest-3) * 24 * time.Hour)

	// Get the table name from the store
	storeImpl := store.(*ibPortsStore)
	purged, err := purge(ctx, dbRW, storeImpl.historyTable, purgeBeforeTime.Unix(), false)
	if err != nil {
		t.Fatalf("failed to purge data: %v", err)
	}
	t.Logf("Purged %d rows (data older than %s)", purged, purgeBeforeTime.Format(time.RFC3339))

	// Measure database size after purging but before compaction
	size, err = sqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read db size after purge: %v", err)
	}
	t.Logf("DB size after purge (before compaction): %s", humanize.Bytes(size))

	// Compact the database again after purging
	if err := sqlite.Compact(ctx, dbRW); err != nil {
		t.Fatalf("failed to compact db after purge: %v", err)
	}

	// Measure final database size after purging and compaction
	size, err = sqlite.ReadDBSize(ctx, dbRO)
	if err != nil {
		t.Fatalf("failed to read db size after purge and compaction: %v", err)
	}
	t.Logf("Final DB size after purge and compaction: %s", humanize.Bytes(size))

	// Test event scanning functionality with realistic data
	t.Logf("Running event scanning to detect drops and flaps...")
	if err := store.Scan(); err != nil {
		t.Fatalf("failed to scan for events: %v", err)
	}

	// Query events detected
	events, err := store.Events(now)
	if err != nil {
		t.Fatalf("failed to query events: %v", err)
	}
	t.Logf("Detected %d events (drops/flaps) in the dataset", len(events))

	// Analyze event types
	eventTypeCounts := make(map[string]int)
	for _, event := range events {
		eventTypeCounts[event.EventType]++
	}

	for eventType, count := range eventTypeCounts {
		t.Logf("Event type '%s': %d occurrences", eventType, count)
	}

	t.Logf("Benchmark completed: %d days of realistic cluster data with %d IB ports ingested every %d seconds",
		daysToIngest, len(deviceNames), intervalSeconds)
	t.Logf("Storage efficiency: Purged %d rows, final storage size: %s", purged, humanize.Bytes(size))
	t.Logf("Event detection: Found %d total events across all ports", len(events))
}

// portProfile represents the profile configuration for a port in realistic simulation
type portProfile struct {
	isHealthy         bool
	baseState         string
	basePhysicalState string
	baseRate          int
	baseLinkDownCount uint64
	failureRate       float32 // probability of temporary issues
	recoveryRate      float32 // probability of recovering from issues
}

// countHealthyPorts counts the number of healthy ports in the profile slice
func countHealthyPorts(profiles []portProfile) int {
	count := 0
	for _, profile := range profiles {
		if profile.isHealthy {
			count++
		}
	}
	return count
}

// openTestDBForBenchmark creates a test database for benchmarks
func openTestDBForBenchmark(b *testing.B) (*sql.DB, *sql.DB, func()) {
	// Create a temporary database file
	tempDir := b.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Open read-write connection
	dbRW, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		b.Fatalf("failed to open RW database: %v", err)
	}

	// Open read-only connection
	dbRO, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&mode=ro")
	if err != nil {
		dbRW.Close()
		b.Fatalf("failed to open RO database: %v", err)
	}

	cleanup := func() {
		dbRW.Close()
		dbRO.Close()
	}

	return dbRW, dbRO, cleanup
}
