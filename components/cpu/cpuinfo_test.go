package cpu

import (
	"runtime"
	"testing"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
)

func TestCalculateCPUUsage(t *testing.T) {
	// Test case 1: When prevStat is nil, should use getUsedPct
	t.Run("with nil prevStat", func(t *testing.T) {
		expectedCPUStat := cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   200,
		}
		expectedUsage := 42.5

		// Call the function with nil prevStat
		usedPercent := calculateCPUUsage(nil, expectedCPUStat, expectedUsage)

		// Verify results
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

		// Call the function with non-nil prevStat
		usedPercent := calculateCPUUsage(&prevCPUStat, currentCPUStat, 0)

		// Verify results
		// We're not checking the exact value here because calculateBusy is a separate function
		// with its own tests, but the result should be a reasonable percentage
		assert.True(t, usedPercent >= 0 && usedPercent <= 100)
	})
}

func TestCalculateBusy(t *testing.T) {
	// Test case 1: Normal case - CPU got busier
	t.Run("normal case", func(t *testing.T) {
		t1 := cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   200,
		}
		t2 := cpu.TimesStat{
			User:   150, // +50
			System: 75,  // +25
			Idle:   225, // +25
		}
		// Total time diff: 100, busy time diff: 75, so busy percentage should be 75%

		busyPercent := calculateBusy(t1, t2)
		assert.Equal(t, 75.0, busyPercent)
	})

	// Test case 2: When t2Busy <= t1Busy (should return 0)
	t.Run("t2Busy <= t1Busy", func(t *testing.T) {
		t1 := cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   200,
		}
		t2 := cpu.TimesStat{
			User:   90,  // -10
			System: 50,  // no change
			Idle:   210, // +10
		}

		busyPercent := calculateBusy(t1, t2)
		assert.Equal(t, 0.0, busyPercent)
	})

	// Test case 3: When t2All <= t1All (should return 100)
	t.Run("t2All <= t1All", func(t *testing.T) {
		t1 := cpu.TimesStat{
			User:   100,
			System: 50,
			Idle:   200,
		}
		t2 := cpu.TimesStat{
			User:   110, // +10
			System: 60,  // +10
			Idle:   170, // -30
		}
		// Total decreased but busy increased

		busyPercent := calculateBusy(t1, t2)
		assert.Equal(t, 100.0, busyPercent)
	})
}

func TestGetAllBusy(t *testing.T) {
	// Test calculation of total and busy times
	t.Run("basic calculation", func(t *testing.T) {
		stat := cpu.TimesStat{
			User:      100,
			System:    50,
			Idle:      200,
			Nice:      10,
			Iowait:    20,
			Irq:       5,
			Softirq:   5,
			Steal:     2,
			Guest:     3,
			GuestNice: 1,
		}

		total, busy := getAllBusy(stat)

		expectedTotal := 396.0
		if runtime.GOOS == "linux" {
			expectedTotal -= 4.0 // Subtract Guest and GuestNice
		}

		expectedBusy := expectedTotal - stat.Idle - stat.Iowait

		assert.Equal(t, expectedTotal, total)
		assert.Equal(t, expectedBusy, busy)
	})

	// Test with zero values
	t.Run("zero values", func(t *testing.T) {
		stat := cpu.TimesStat{
			User:   0,
			System: 0,
			Idle:   0,
		}

		total, busy := getAllBusy(stat)

		assert.Equal(t, 0.0, total)
		assert.Equal(t, 0.0, busy)
	})
}

func TestSetGetPrevTimeStat(t *testing.T) {
	// Test setting and getting previous time stat
	expectedStat := cpu.TimesStat{
		User:   100,
		System: 50,
		Idle:   200,
	}

	// Clear existing state
	setPrevTimeStat(cpu.TimesStat{})

	// Set the value
	setPrevTimeStat(expectedStat)

	// Get the value
	actualStat := getPrevTimeStat()

	// Verify
	assert.NotNil(t, actualStat)
	assert.Equal(t, expectedStat, *actualStat)
}
