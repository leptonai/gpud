package nvlink

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
			if got != tc.expected {
				t.Fatalf("IsZero() for value %d: expected %v, got %v", tc.value, tc.expected, got)
			}
		})
	}
}

func TestDefaultExpectedLinkStates(t *testing.T) {
	original := GetDefaultExpectedLinkStates()
	defer SetDefaultExpectedLinkStates(original)

	custom := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: 4}
	SetDefaultExpectedLinkStates(custom)

	got := GetDefaultExpectedLinkStates()
	if got != custom {
		t.Fatalf("expected %+v, got %+v", custom, got)
	}
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

	// Start multiple writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				value := id*100 + j
				SetDefaultExpectedLinkStates(ExpectedLinkStates{
					AtLeastGPUsWithAllLinksFeatureEnabled: value,
				})
				atomic.AddInt32(&writeCount, 1)
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Start multiple readers that verify consistency
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				val := GetDefaultExpectedLinkStates()

				// All read values should be non-negative (validation should handle negatives)
				if val.AtLeastGPUsWithAllLinksFeatureEnabled < 0 {
					t.Errorf("read negative value during concurrent access: %d",
						val.AtLeastGPUsWithAllLinksFeatureEnabled)
				}

				// Collect read values for analysis
				readMu.Lock()
				readValues = append(readValues, val.AtLeastGPUsWithAllLinksFeatureEnabled)
				readMu.Unlock()

				time.Sleep(time.Microsecond)
			}
		}()
	}

	// Start writers with negative values to test validation under concurrency
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations/2; j++ {
				SetDefaultExpectedLinkStates(ExpectedLinkStates{
					AtLeastGPUsWithAllLinksFeatureEnabled: -(id*10 + j),
				})
				time.Sleep(time.Microsecond)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify we can still read without panic
	final := GetDefaultExpectedLinkStates()
	if final.AtLeastGPUsWithAllLinksFeatureEnabled < 0 {
		t.Fatalf("final value should never be negative after validation: %d",
			final.AtLeastGPUsWithAllLinksFeatureEnabled)
	}

	// Verify all read values were non-negative
	for i, val := range readValues {
		if val < 0 {
			t.Errorf("read value %d at index %d was negative, validation failed", val, i)
		}
	}

	// Verify we actually performed reads and writes
	if len(readValues) == 0 {
		t.Error("no values were read during concurrent access test")
	}
	if writeCount == 0 {
		t.Error("no writes were performed during concurrent access test")
	}

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
			if got.AtLeastGPUsWithAllLinksFeatureEnabled != tc.expected {
				t.Errorf("expected %d, got %d", tc.expected, got.AtLeastGPUsWithAllLinksFeatureEnabled)
			}
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
			if got.AtLeastGPUsWithAllLinksFeatureEnabled != tc.expected {
				t.Errorf("negative value %d should be treated as %d, got %d", tc.value, tc.expected, got.AtLeastGPUsWithAllLinksFeatureEnabled)
			}

			// Verify IsZero returns true for sanitized negative values (now 0)
			if !got.IsZero() {
				t.Errorf("expected IsZero() to return true after negative value sanitization to 0")
			}

			// Also test that the original negative value is treated as unset
			negativeState := ExpectedLinkStates{AtLeastGPUsWithAllLinksFeatureEnabled: tc.value}
			if !negativeState.IsZero() {
				t.Errorf("expected IsZero() to return true for negative value %d", tc.value)
			}
		})
	}
}
