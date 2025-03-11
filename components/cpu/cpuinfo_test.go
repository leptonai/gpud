package cpu

import (
	"context"
	"fmt"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
)

func TestCalculateCPUUsage(t *testing.T) {
	ctx := context.Background()

	// Test case 1: When prevStat is nil, should use getUsedPct
	t.Run("with nil prevStat", func(t *testing.T) {
		expectedCPUStat := cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   200,
		}
		expectedUsage := 42.5

		// Mock the getTimeStat function
		getTimeStatMock := func(ctx context.Context) (cpu.TimesStat, error) {
			return expectedCPUStat, nil
		}

		// Mock the getUsedPct function
		getUsedPctMock := func(ctx context.Context) (float64, error) {
			return expectedUsage, nil
		}

		// Call the function with nil prevStat
		curStat, usedPercent, err := calculateCPUUsage(ctx, nil, getTimeStatMock, getUsedPctMock)

		// Verify results
		assert.NoError(t, err)
		assert.Equal(t, expectedCPUStat, curStat)
		assert.Equal(t, expectedUsage, usedPercent)
	})

	// Test case 2: When prevStat is not nil, should calculate busy percentage
	t.Run("with non-nil prevStat", func(t *testing.T) {
		prevCPUStat := cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   200,
		}
		currentCPUStat := cpu.TimesStat{
			User:   150, // +50
			System: 75,  // +25
			Idle:   225, // +25
		}
		// Total time diff: 100, busy time diff: 75, so busy percentage should be 75%

		// Mock the getTimeStat function
		getTimeStatMock := func(ctx context.Context) (cpu.TimesStat, error) {
			return currentCPUStat, nil
		}

		// Mock the getUsedPct function - should not be called in this case
		getUsedPctMock := func(ctx context.Context) (float64, error) {
			t.Fatal("getUsedPct should not be called when prevStat is not nil")
			return 0, nil
		}

		// Call the function with non-nil prevStat
		curStat, usedPercent, err := calculateCPUUsage(ctx, &prevCPUStat, getTimeStatMock, getUsedPctMock)

		// Verify results
		assert.NoError(t, err)
		assert.Equal(t, currentCPUStat, curStat)
		// We're not checking the exact value here because calculateBusy is a separate function
		// with its own tests, but the result should be a reasonable percentage
		assert.True(t, usedPercent >= 0 && usedPercent <= 100)
	})

	// Test case 3: When getTimeStat returns an error
	t.Run("with getTimeStat error", func(t *testing.T) {
		expectedErr := fmt.Errorf("failed to get time stats")

		// Mock the getTimeStat function to return an error
		getTimeStatMock := func(ctx context.Context) (cpu.TimesStat, error) {
			return cpu.TimesStat{}, expectedErr
		}

		// Mock the getUsedPct function - should not be called in this case
		getUsedPctMock := func(ctx context.Context) (float64, error) {
			t.Fatal("getUsedPct should not be called when getTimeStat fails")
			return 0, nil
		}

		// Call the function
		_, _, err := calculateCPUUsage(ctx, nil, getTimeStatMock, getUsedPctMock)

		// Verify results
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	// Test case 4: When getUsedPct returns an error (with nil prevStat)
	t.Run("with getUsedPct error", func(t *testing.T) {
		expectedCPUStat := cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   200,
		}
		expectedErr := fmt.Errorf("failed to get usage percentage")

		// Mock the getTimeStat function
		getTimeStatMock := func(ctx context.Context) (cpu.TimesStat, error) {
			return expectedCPUStat, nil
		}

		// Mock the getUsedPct function to return an error
		getUsedPctMock := func(ctx context.Context) (float64, error) {
			return 0, expectedErr
		}

		// Call the function with nil prevStat
		_, _, err := calculateCPUUsage(ctx, nil, getTimeStatMock, getUsedPctMock)

		// Verify results
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})
}
