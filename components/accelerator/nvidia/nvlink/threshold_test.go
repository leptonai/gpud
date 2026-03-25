package nvlink

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExpectedLinkStates_IsZero(t *testing.T) {
	testCases := []struct {
		name     string
		value    int
		expected bool
	}{
		{"zero", 0, true},
		{"negative -1", -1, true},
		{"negative -100", -100, true},
		{"positive 1", 1, false},
		{"positive 2", 2, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: tc.value}
			got := states.IsZero()
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestDefaultExpectedLinkStates(t *testing.T) {
	original := GetDefaultExpectedLinkStates()
	defer SetDefaultExpectedLinkStates(original)

	custom := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 4}
	SetDefaultExpectedLinkStates(custom)

	got := GetDefaultExpectedLinkStates()
	assert.Equal(t, custom, got)
}

func TestConcurrentAccess(t *testing.T) {
	// Save original state
	original := GetDefaultExpectedLinkStates()
	defer func() {
		SetDefaultExpectedLinkStates(original)
	}()

	// Test concurrent reads and writes to ensure thread safety
	var wg sync.WaitGroup
	iterations := 100

	// Track all read values for consistency verification
	var readValues []int
	var readMu sync.Mutex

	// Track successful writes
	var writeCount int32
	var negativeReadCount int32

	// Start multiple writers
	for i := range 5 {
		wg.Go(func() {
			for j := range iterations {
				value := i*100 + j
				SetDefaultExpectedLinkStates(ExpectedLinkStates{
					AtLeastGPUsWithAllLinksFeatureEnabled: value,
				})
				atomic.AddInt32(&writeCount, 1)
				time.Sleep(time.Microsecond)
			}
		})
	}

	// Start multiple readers that verify consistency
	for range 5 {
		wg.Go(func() {
			for range iterations {
				val := GetDefaultExpectedLinkStates()

				if val.AtLeastGPUsWithAllLinksFeatureEnabled < 0 {
					atomic.AddInt32(&negativeReadCount, 1)
				}

				// Collect read values for analysis
				readMu.Lock()
				readValues = append(readValues, val.AtLeastGPUsWithAllLinksFeatureEnabled)
				readMu.Unlock()

				time.Sleep(time.Microsecond)
			}
		})
	}

	// Start writers with negative values to test validation under concurrency
	for i := range 2 {
		wg.Go(func() {
			for j := range iterations / 2 {
				SetDefaultExpectedLinkStates(ExpectedLinkStates{
					AtLeastGPUsWithAllLinksFeatureEnabled: -(i*10 + j),
				})
				time.Sleep(time.Microsecond)
			}
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify we can still read without panic
	final := GetDefaultExpectedLinkStates()
	assert.GreaterOrEqual(t, final.AtLeastGPUsWithAllLinksFeatureEnabled, 0)
	assert.Zero(t, negativeReadCount)

	// Verify all read values were non-negative
	for i, val := range readValues {
		assert.GreaterOrEqualf(t, val, 0, "read value at index %d was negative", i)
	}

	// Verify we actually performed reads and writes
	assert.NotEmpty(t, readValues)
	assert.NotZero(t, writeCount)

	t.Logf("Concurrent test completed: %d writes, %d reads, final value: %d",
		writeCount, len(readValues), final.AtLeastGPUsWithAllLinksFeatureEnabled)
}

func TestSetDefaultExpectedLinkStates_MultipleValues(t *testing.T) {
	// Save original state
	original := GetDefaultExpectedLinkStates()
	defer func() {
		SetDefaultExpectedLinkStates(original)
	}()

	testCases := []struct {
		name     string
		value    int
		expected int
	}{
		{"zero", 0, 0},
		{"single GPU", 1, 1},
		{"eight GPUs", 8, 8},
		{"sixteen GPUs", 16, 16},
		{"large value", 1000, 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states := ExpectedLinkStates{
				AtLeastGPUsWithAllLinksFeatureEnabled: tc.value,
			}
			SetDefaultExpectedLinkStates(states)

			got := GetDefaultExpectedLinkStates()
			assert.Equal(t, tc.expected, got.AtLeastGPUsWithAllLinksFeatureEnabled)
		})
	}
}

func TestSetDefaultExpectedLinkStates_NegativeValues(t *testing.T) {
	// Save original state
	original := GetDefaultExpectedLinkStates()
	defer func() {
		SetDefaultExpectedLinkStates(original)
	}()

	testCases := []struct {
		name     string
		value    int
		expected int
	}{
		{"negative -1", -1, 0},
		{"negative -100", -100, 0},
		{"negative min int", -2147483648, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			states := ExpectedLinkStates{
				AtLeastGPUsWithAllLinksFeatureEnabled: tc.value,
			}
			SetDefaultExpectedLinkStates(states)

			got := GetDefaultExpectedLinkStates()
			assert.Equal(t, tc.expected, got.AtLeastGPUsWithAllLinksFeatureEnabled)

			// Verify IsZero returns true for sanitized negative values (now 0)
			assert.True(t, got.IsZero())

			// Also test that the original negative value is treated as unset
			negativeState := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: tc.value}
			assert.True(t, negativeState.IsZero())
		})
	}
}
