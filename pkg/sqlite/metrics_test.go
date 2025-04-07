package sqlite

import (
	"math"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()

	require.NoError(t, reg.Register(metricInsertUpdateTotal))
	require.NoError(t, reg.Register(metricInsertUpdateSecondsTotal))
	require.NoError(t, reg.Register(metricDeleteTotal))
	require.NoError(t, reg.Register(metricDeleteSecondsTotal))
	require.NoError(t, reg.Register(metricSelectTotal))
	require.NoError(t, reg.Register(metricSelectSecondsTotal))

	mtr, err := ReadMetrics(reg)
	require.NoError(t, err)
	require.True(t, mtr.IsZero(), "initial metrics should be zero")
	require.Equal(t, int64(0), mtr.InsertUpdateTotal, "initial insert/update total should be 0")
	require.Equal(t, float64(0), mtr.InsertUpdateSecondsTotal, "initial insert/update seconds total should be 0")
	require.Equal(t, int64(0), mtr.DeleteTotal, "initial delete total should be 0")
	require.Equal(t, float64(0), mtr.DeleteSecondsTotal, "initial delete seconds total should be 0")
	require.Equal(t, int64(0), mtr.SelectTotal, "initial select total should be 0")
	require.Equal(t, float64(0), mtr.SelectSecondsTotal, "initial select seconds total should be 0")
	require.Equal(t, float64(0), mtr.SelectSecondsAvg, "initial select seconds avg should be 0")

	const (
		inserts          = 10
		secondsPerInsert = float64(0.7)
	)
	expectedSecondsWrites := float64(inserts) * secondsPerInsert

	for i := 0; i < inserts; i++ {
		RecordInsertUpdate(secondsPerInsert)
	}

	mtr, err = ReadMetrics(reg)
	require.NoError(t, err, "failed to read insert/update metrics")
	require.Equal(t, int64(inserts), mtr.InsertUpdateTotal, "expected %d inserts", inserts)
	require.True(t, floatEquals(mtr.InsertUpdateSecondsTotal, expectedSecondsWrites),
		"expected %.3f seconds total for inserts, got %.3f", expectedSecondsWrites, mtr.InsertUpdateSecondsTotal)
	require.True(t, floatEquals(mtr.InsertUpdateSecondsAvg, secondsPerInsert),
		"expected %.3f seconds avg for inserts, got %.3f", secondsPerInsert, mtr.InsertUpdateSecondsAvg)

	const (
		deletes          = 5
		secondsPerDelete = float64(0.9)
	)
	expectedSecondsDeletes := float64(deletes) * secondsPerDelete
	for i := 0; i < deletes; i++ {
		RecordDelete(secondsPerDelete)
	}

	mtr, err = ReadMetrics(reg)
	require.NoError(t, err, "failed to read delete metrics")
	require.Equal(t, int64(deletes), mtr.DeleteTotal, "expected %d deletes", deletes)
	require.True(t, floatEquals(mtr.DeleteSecondsTotal, expectedSecondsDeletes),
		"expected %.3f seconds total for deletes, got %.3f", expectedSecondsDeletes, mtr.DeleteSecondsTotal)
	require.True(t, floatEquals(mtr.DeleteSecondsAvg, secondsPerDelete),
		"expected %.3f seconds avg for deletes, got %.3f", secondsPerDelete, mtr.DeleteSecondsAvg)

	const (
		selects       = 20
		secsPerSelect = 0.50
	)
	expectedSecondsSelect := float64(selects) * secsPerSelect

	for i := 0; i < selects; i++ {
		RecordSelect(secsPerSelect)
	}

	mtr, err = ReadMetrics(reg)
	require.NoError(t, err, "failed to read select metrics")

	require.Equal(t, int64(selects), mtr.SelectTotal, "expected %d selects, got %d", selects, mtr.SelectTotal)
	require.True(t, floatEquals(mtr.SelectSecondsTotal, expectedSecondsSelect),
		"expected %.3f seconds total for selects, got %.3f", expectedSecondsSelect, mtr.SelectSecondsTotal)
	require.Equal(t, int64(inserts), mtr.InsertUpdateTotal, "insert count changed unexpectedly: expected %d, got %d", inserts, mtr.InsertUpdateTotal)
	require.True(t, floatEquals(mtr.InsertUpdateSecondsTotal, expectedSecondsWrites),
		"insert seconds changed unexpectedly: expected %.3f, got %.3f", expectedSecondsWrites, mtr.InsertUpdateSecondsTotal)
	require.True(t, floatEquals(mtr.DeleteSecondsAvg, secondsPerDelete),
		"delete seconds avg changed unexpectedly: expected %.3f, got %.3f", secondsPerDelete, mtr.DeleteSecondsAvg)
	require.True(t, floatEquals(mtr.SelectSecondsAvg, secsPerSelect),
		"select seconds avg changed unexpectedly: expected %.3f, got %.3f", secsPerSelect, mtr.SelectSecondsAvg)
}

func TestMetricsIsZero(t *testing.T) {
	tests := []struct {
		name     string
		metrics  Metrics
		expected bool
	}{
		{
			name:     "zero metrics",
			metrics:  Metrics{},
			expected: true,
		},
		{
			name: "non-zero insert update",
			metrics: Metrics{
				InsertUpdateTotal: 1,
			},
			expected: false,
		},
		{
			name: "non-zero insert update seconds",
			metrics: Metrics{
				InsertUpdateSecondsTotal: 0.1,
			},
			expected: false,
		},
		{
			name: "non-zero insert update seconds avg",
			metrics: Metrics{
				InsertUpdateSecondsAvg: 0.1,
			},
			expected: false,
		},
		{
			name: "non-zero delete total",
			metrics: Metrics{
				DeleteTotal: 1,
			},
			expected: false,
		},
		{
			name: "non-zero delete seconds",
			metrics: Metrics{
				DeleteSecondsTotal: 0.1,
			},
			expected: false,
		},
		{
			name: "non-zero delete seconds avg",
			metrics: Metrics{
				DeleteSecondsAvg: 0.1,
			},
			expected: false,
		},
		{
			name: "non-zero select total",
			metrics: Metrics{
				SelectTotal: 1,
			},
			expected: false,
		},
		{
			name: "non-zero select seconds",
			metrics: Metrics{
				SelectSecondsTotal: 0.1,
			},
			expected: false,
		},
		{
			name: "non-zero select seconds avg",
			metrics: Metrics{
				SelectSecondsAvg: 0.1,
			},
			expected: false,
		},
		{
			name: "only time set",
			metrics: Metrics{
				Time: time.Now(),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.metrics.IsZero(); got != tt.expected {
				t.Errorf("IsZero() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < 0.0005
}

func TestCalculateQPS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		lastMetrics         Metrics
		currMetrics         Metrics
		wantInsertUpdateQPS float64
		wantDeleteQPS       float64
		wantSelectQPS       float64
	}{
		{
			name:                "both metrics zero",
			lastMetrics:         Metrics{},
			currMetrics:         Metrics{},
			wantInsertUpdateQPS: 0,
			wantDeleteQPS:       0,
			wantSelectQPS:       0,
		},
		{
			name: "normal case with 10 second interval",
			lastMetrics: Metrics{
				Time:              time.Unix(1000, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			currMetrics: Metrics{
				Time:              time.Unix(1010, 0),
				InsertUpdateTotal: 200,
				DeleteTotal:       70,
				SelectTotal:       400,
			},
			wantInsertUpdateQPS: 10, // (200-100)/10
			wantDeleteQPS:       2,  // (70-50)/10
			wantSelectQPS:       20, // (400-200)/10
		},
		{
			name:        "last metrics zero",
			lastMetrics: Metrics{},
			currMetrics: Metrics{
				Time:              time.Unix(1000, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			wantInsertUpdateQPS: 0,
			wantDeleteQPS:       0,
			wantSelectQPS:       0,
		},
		{
			name: "current metrics zero",
			lastMetrics: Metrics{
				Time:              time.Unix(1000, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			currMetrics:         Metrics{},
			wantInsertUpdateQPS: 0,
			wantDeleteQPS:       0,
			wantSelectQPS:       0,
		},
		{
			name: "sub-second interval",
			lastMetrics: Metrics{
				Time:              time.Unix(1000, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			currMetrics: Metrics{
				Time:              time.Unix(1000, 500000000), // 500ms later
				InsertUpdateTotal: 150,
				DeleteTotal:       75,
				SelectTotal:       300,
			},
			wantInsertUpdateQPS: 100, // (150-100)/0.5
			wantDeleteQPS:       50,  // (75-50)/0.5
			wantSelectQPS:       200, // (300-200)/0.5
		},
		{
			name: "no changes in counts",
			lastMetrics: Metrics{
				Time:              time.Unix(1000, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			currMetrics: Metrics{
				Time:              time.Unix(1010, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			wantInsertUpdateQPS: 0,
			wantDeleteQPS:       0,
			wantSelectQPS:       0,
		},
		{
			name: "very short time interval",
			lastMetrics: Metrics{
				Time:              time.Unix(1000, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			currMetrics: Metrics{
				Time:              time.Unix(1000, 1000000), // 1ms later
				InsertUpdateTotal: 101,
				DeleteTotal:       51,
				SelectTotal:       201,
			},
			wantInsertUpdateQPS: 1000, // (101-100)/0.001
			wantDeleteQPS:       1000, // (51-50)/0.001
			wantSelectQPS:       1000, // (201-200)/0.001
		},
		{
			name: "mixed activity - some metrics changing, others not",
			lastMetrics: Metrics{
				Time:              time.Unix(1000, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			currMetrics: Metrics{
				Time:              time.Unix(1010, 0),
				InsertUpdateTotal: 100, // no change
				DeleteTotal:       70,  // changed
				SelectTotal:       400, // changed
			},
			wantInsertUpdateQPS: 0,  // (100-100)/10
			wantDeleteQPS:       2,  // (70-50)/10
			wantSelectQPS:       20, // (400-200)/10
		},
		{
			name: "zero elapsed time - identical timestamps",
			lastMetrics: Metrics{
				Time:              time.Unix(1000, 0),
				InsertUpdateTotal: 100,
				DeleteTotal:       50,
				SelectTotal:       200,
			},
			currMetrics: Metrics{
				Time:              time.Unix(1000, 0), // same timestamp
				InsertUpdateTotal: 150,
				DeleteTotal:       75,
				SelectTotal:       300,
			},
			wantInsertUpdateQPS: 0, // should return 0 when elapsed time is 0
			wantDeleteQPS:       0,
			wantSelectQPS:       0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotInsertUpdateQPS, gotDeleteQPS, gotSelectQPS := tt.lastMetrics.QPS(tt.currMetrics)

			if gotInsertUpdateQPS != tt.wantInsertUpdateQPS {
				t.Errorf("calculateMetrics() gotInsertUpdateQPS = %v, want %v", gotInsertUpdateQPS, tt.wantInsertUpdateQPS)
			}
			if gotDeleteQPS != tt.wantDeleteQPS {
				t.Errorf("calculateMetrics() gotDeleteQPS = %v, want %v", gotDeleteQPS, tt.wantDeleteQPS)
			}
			if gotSelectQPS != tt.wantSelectQPS {
				t.Errorf("calculateMetrics() gotSelectQPS = %v, want %v", gotSelectQPS, tt.wantSelectQPS)
			}
		})
	}
}
