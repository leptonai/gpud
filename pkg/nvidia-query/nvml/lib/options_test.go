package lib

import (
	"testing"

	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
)

// TestApplyOptsDefault tests the default behavior of applyOpts when no options are provided
func TestApplyOptsDefault(t *testing.T) {
	// Create an empty Op
	op := &Op{}

	// Apply no options
	op.applyOpts([]OpOption{})

	// Verify that nvmlLib is set to a non-nil value (default nvml.New())
	assert.NotNil(t, op.nvmlLib, "nvmlLib should be set to a default value when no options are provided")
}

// TestWithNVML tests that WithNVML correctly sets the nvmlLib field
func TestWithNVML(t *testing.T) {
	// Create a mock NVML interface
	mockNVML := &mock.Interface{}

	// Create an empty Op
	op := &Op{}

	// Apply the WithNVML option
	op.applyOpts([]OpOption{WithNVML(mockNVML)})

	// Verify that nvmlLib is set to our mock
	assert.Equal(t, mockNVML, op.nvmlLib, "nvmlLib should be set to the provided mock")
}

// TestWithInitReturn tests that WithInitReturn correctly sets the initReturn field
func TestWithInitReturn(t *testing.T) {
	// Create an empty Op
	op := &Op{}

	// Test value
	testReturn := nvml.ERROR_UNKNOWN

	// Apply the WithInitReturn option
	op.applyOpts([]OpOption{WithInitReturn(testReturn)})

	// Verify that initReturn is set and points to our test value
	assert.NotNil(t, op.initReturn, "initReturn should not be nil")
	assert.Equal(t, testReturn, *op.initReturn, "initReturn should be set to the provided value")
}

// TestWithPropertyExtractor tests that WithPropertyExtractor correctly sets the propertyExtractor field
func TestWithPropertyExtractor(t *testing.T) {
	// Create a mock PropertyExtractor
	mockExtractor := &nvinfo.PropertyExtractorMock{}

	// Create an empty Op
	op := &Op{}

	// Apply the WithPropertyExtractor option
	op.applyOpts([]OpOption{WithPropertyExtractor(mockExtractor)})

	// Verify that propertyExtractor is set to our mock
	assert.Equal(t, mockExtractor, op.propertyExtractor, "propertyExtractor should be set to the provided mock")
}

// TestWithDeviceGetRemappedRowsForAllDevs tests that WithDeviceGetRemappedRowsForAllDevs correctly sets the function
func TestWithDeviceGetRemappedRowsForAllDevs(t *testing.T) {
	// Create a test function
	testFunc := func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return) {
		return 1, 2, true, false, nvml.SUCCESS
	}

	// Create an empty Op
	op := &Op{}

	// Apply the WithDeviceGetRemappedRowsForAllDevs option
	op.applyOpts([]OpOption{WithDeviceGetRemappedRowsForAllDevs(testFunc)})

	// Verify that the function is set
	assert.NotNil(t, op.devGetRemappedRowsForAllDevs, "devGetRemappedRowsForAllDevs should be set")

	// Call the function and verify it returns the expected values
	corrRows, uncRows, isPending, failureOccurred, ret := op.devGetRemappedRowsForAllDevs()
	assert.Equal(t, 1, corrRows, "corrRows should match")
	assert.Equal(t, 2, uncRows, "uncRows should match")
	assert.True(t, isPending, "isPending should match")
	assert.False(t, failureOccurred, "failureOccurred should match")
	assert.Equal(t, nvml.SUCCESS, ret, "ret should match")
}

// TestWithDeviceGetCurrentClocksEventReasonsForAllDevs tests that WithDeviceGetCurrentClocksEventReasonsForAllDevs correctly sets the function
func TestWithDeviceGetCurrentClocksEventReasonsForAllDevs(t *testing.T) {
	// Create a test function
	testFunc := func() (uint64, nvml.Return) {
		return 42, nvml.SUCCESS
	}

	// Create an empty Op
	op := &Op{}

	// Apply the WithDeviceGetCurrentClocksEventReasonsForAllDevs option
	op.applyOpts([]OpOption{WithDeviceGetCurrentClocksEventReasonsForAllDevs(testFunc)})

	// Verify that the function is set
	assert.NotNil(t, op.devGetCurrentClocksEventReasonsForAllDevs, "devGetCurrentClocksEventReasonsForAllDevs should be set")

	// Call the function and verify it returns the expected values
	reasons, ret := op.devGetCurrentClocksEventReasonsForAllDevs()
	assert.Equal(t, uint64(42), reasons, "reasons should match")
	assert.Equal(t, nvml.SUCCESS, ret, "ret should match")
}

// TestMultipleOptions tests that multiple options can be applied correctly
func TestMultipleOptions(t *testing.T) {
	// Create mocks and test values
	mockNVML := &mock.Interface{}
	mockExtractor := &nvinfo.PropertyExtractorMock{}
	testReturn := nvml.ERROR_UNKNOWN

	// Create test functions
	remappedRowsFunc := func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return) {
		return 1, 2, true, false, nvml.SUCCESS
	}

	clockEventsFunc := func() (uint64, nvml.Return) {
		return 42, nvml.SUCCESS
	}

	// Create an empty Op
	op := &Op{}

	// Apply all options at once
	op.applyOpts([]OpOption{
		WithNVML(mockNVML),
		WithInitReturn(testReturn),
		WithPropertyExtractor(mockExtractor),
		WithDeviceGetRemappedRowsForAllDevs(remappedRowsFunc),
		WithDeviceGetCurrentClocksEventReasonsForAllDevs(clockEventsFunc),
	})

	// Verify all fields are set correctly
	assert.Equal(t, mockNVML, op.nvmlLib, "nvmlLib should be set correctly")
	assert.NotNil(t, op.initReturn, "initReturn should not be nil")
	assert.Equal(t, testReturn, *op.initReturn, "initReturn should be set correctly")
	assert.Equal(t, mockExtractor, op.propertyExtractor, "propertyExtractor should be set correctly")
	assert.NotNil(t, op.devGetRemappedRowsForAllDevs, "devGetRemappedRowsForAllDevs should be set")
	assert.NotNil(t, op.devGetCurrentClocksEventReasonsForAllDevs, "devGetCurrentClocksEventReasonsForAllDevs should be set")

	// Call the functions and verify they return the expected values
	corrRows, uncRows, isPending, failureOccurred, retRows := op.devGetRemappedRowsForAllDevs()
	assert.Equal(t, 1, corrRows, "corrRows should match")
	assert.Equal(t, 2, uncRows, "uncRows should match")
	assert.True(t, isPending, "isPending should match")
	assert.False(t, failureOccurred, "failureOccurred should match")
	assert.Equal(t, nvml.SUCCESS, retRows, "retRows should match")

	reasons, retClock := op.devGetCurrentClocksEventReasonsForAllDevs()
	assert.Equal(t, uint64(42), reasons, "reasons should match")
	assert.Equal(t, nvml.SUCCESS, retClock, "retClock should match")
}
