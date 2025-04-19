package nccl

import (
	"context"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	nvmllib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

// MockEventBucket implements a mock for eventstore.Bucket
type MockEventBucket struct {
	mock.Mock
}

func (m *MockEventBucket) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockEventBucket) Insert(ctx context.Context, event apiv1.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func (m *MockEventBucket) Find(ctx context.Context, event apiv1.Event) (*apiv1.Event, error) {
	args := m.Called(ctx, event)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*apiv1.Event), args.Error(1)
}

func (m *MockEventBucket) Get(ctx context.Context, since time.Time) (apiv1.Events, error) {
	args := m.Called(ctx, since)
	return args.Get(0).(apiv1.Events), args.Error(1)
}

func (m *MockEventBucket) Latest(ctx context.Context) (*apiv1.Event, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*apiv1.Event), args.Error(1)
}

func (m *MockEventBucket) Purge(ctx context.Context, beforeTimestamp int64) (int, error) {
	args := m.Called(ctx, beforeTimestamp)
	return args.Int(0), args.Error(1)
}

func (m *MockEventBucket) Close() {
	m.Called()
}

// MockEventStore implements a mock for eventstore.Store
type MockEventStore struct {
	mock.Mock
}

func (m *MockEventStore) Bucket(name string, opts ...eventstore.OpOption) (eventstore.Bucket, error) {
	args := m.Called(name)
	return args.Get(0).(eventstore.Bucket), args.Error(1)
}

// Complete implementation of nvidianvml.InstanceV2 for testing
type mockNvmlInstance struct {
	mock.Mock
}

func (m *mockNvmlInstance) NVMLExists() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *mockNvmlInstance) Library() nvmllib.Library {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(nvmllib.Library)
}

func (m *mockNvmlInstance) Devices() map[string]device.Device {
	args := m.Called()
	return args.Get(0).(map[string]device.Device)
}

func (m *mockNvmlInstance) ProductName() string {
	args := m.Called()
	return args.String(0)
}

func (m *mockNvmlInstance) GetMemoryErrorManagementCapabilities() nvidianvml.MemoryErrorManagementCapabilities {
	args := m.Called()
	return args.Get(0).(nvidianvml.MemoryErrorManagementCapabilities)
}

func (m *mockNvmlInstance) Shutdown() error {
	args := m.Called()
	return args.Error(0)
}

// TestCheck tests the Check method in various scenarios
func TestCheck(t *testing.T) {
	t.Parallel()

	t.Run("nil nvmlInstance", func(t *testing.T) {
		comp := &component{
			nvmlInstance: nil,
		}
		result := comp.Check()
		assert.NotNil(t, result)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthState())
		assert.Contains(t, result.Summary(), "NVIDIA NVML instance is nil")
	})

	t.Run("nil readAllKmsg", func(t *testing.T) {
		// Mock nvmlInstance that returns true for NVMLExists
		mockNvml := new(mockNvmlInstance)
		mockNvml.On("NVMLExists").Return(true)
		mockNvml.On("Devices").Return(map[string]device.Device{})
		mockNvml.On("GetMemoryErrorManagementCapabilities").Return(nvidianvml.MemoryErrorManagementCapabilities{})

		comp := &component{
			nvmlInstance: mockNvml,
			readAllKmsg:  nil,
		}
		result := comp.Check()
		assert.NotNil(t, result)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthState())
		assert.Contains(t, result.Summary(), "kmsg reader is not set")
	})

	t.Run("readAllKmsg returns error", func(t *testing.T) {
		// Mock nvmlInstance that returns true for NVMLExists
		mockNvml := new(mockNvmlInstance)
		mockNvml.On("NVMLExists").Return(true)
		mockNvml.On("Devices").Return(map[string]device.Device{})
		mockNvml.On("GetMemoryErrorManagementCapabilities").Return(nvidianvml.MemoryErrorManagementCapabilities{})

		// Mock readAllKmsg that returns an error
		mockReadAllKmsg := func(ctx context.Context) ([]kmsg.Message, error) {
			return nil, assert.AnError
		}

		comp := &component{
			ctx:          context.Background(),
			nvmlInstance: mockNvml,
			readAllKmsg:  mockReadAllKmsg,
		}
		result := comp.Check()
		assert.NotNil(t, result)
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, result.HealthState())
		assert.Contains(t, result.Summary(), "failed to read kmsg")
	})

	t.Run("no matching messages", func(t *testing.T) {
		// Mock nvmlInstance that returns true for NVMLExists
		mockNvml := new(mockNvmlInstance)
		mockNvml.On("NVMLExists").Return(true)
		mockNvml.On("Devices").Return(map[string]device.Device{})
		mockNvml.On("GetMemoryErrorManagementCapabilities").Return(nvidianvml.MemoryErrorManagementCapabilities{})

		// Mock readAllKmsg that returns messages but none match
		mockReadAllKmsg := func(ctx context.Context) ([]kmsg.Message, error) {
			return []kmsg.Message{
				{
					Message: "non-matching message",
				},
			}, nil
		}

		comp := &component{
			ctx:          context.Background(),
			nvmlInstance: mockNvml,
			readAllKmsg:  mockReadAllKmsg,
		}
		result := comp.Check()
		assert.NotNil(t, result)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthState())
		assert.Contains(t, result.Summary(), "matched 0 kmsg(s)")
	})

	t.Run("with matching messages", func(t *testing.T) {
		// Mock nvmlInstance that returns true for NVMLExists
		mockNvml := new(mockNvmlInstance)
		mockNvml.On("NVMLExists").Return(true)
		mockNvml.On("Devices").Return(map[string]device.Device{})
		mockNvml.On("GetMemoryErrorManagementCapabilities").Return(nvidianvml.MemoryErrorManagementCapabilities{})

		// Mock readAllKmsg that returns messages with NCCL errors
		mockReadAllKmsg := func(ctx context.Context) ([]kmsg.Message, error) {
			return []kmsg.Message{
				{
					Message: "non-matching message",
				},
				{
					Message: "segfault at 123 in libnccl.so.2",
				},
			}, nil
		}

		comp := &component{
			ctx:          context.Background(),
			nvmlInstance: mockNvml,
			readAllKmsg:  mockReadAllKmsg,
		}
		result := comp.Check()
		assert.NotNil(t, result)
		assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthState())
		assert.Contains(t, result.Summary(), "matched 1 kmsg(s)")

		data, ok := result.(*Data)
		assert.True(t, ok)
		assert.Len(t, data.MatchedKmsgs, 1)
		assert.Equal(t, "segfault at 123 in libnccl.so.2", data.MatchedKmsgs[0].Message)
	})
}

// TestDataMethods tests the methods of the Data struct
func TestDataMethods(t *testing.T) {
	t.Parallel()

	t.Run("nil data", func(t *testing.T) {
		var d *Data
		assert.Equal(t, "", d.String())
		assert.Equal(t, "", d.Summary())
		assert.Equal(t, apiv1.HealthStateType(""), d.HealthState())
	})

	t.Run("empty data", func(t *testing.T) {
		d := &Data{
			health: apiv1.HealthStateTypeHealthy,
			reason: "test reason",
		}
		assert.Equal(t, "matched 0 kmsg(s)", d.String())
		assert.Equal(t, "test reason", d.Summary())
		assert.Equal(t, apiv1.HealthStateTypeHealthy, d.HealthState())
	})

	t.Run("data with matched kmsg(s)", func(t *testing.T) {
		d := &Data{
			MatchedKmsgs: []kmsg.Message{
				{Message: "test message 1"},
				{Message: "test message 2"},
			},
			health: apiv1.HealthStateTypeUnhealthy,
			reason: "matched kmsg(s)",
		}
		assert.Equal(t, "matched 2 kmsg(s)", d.String())
		assert.Equal(t, "matched kmsg(s)", d.Summary())
		assert.Equal(t, apiv1.HealthStateTypeUnhealthy, d.HealthState())
	})
}

func TestComponentName(t *testing.T) {
	t.Parallel()

	comp := &component{}
	assert.Equal(t, Name, comp.Name())
}

func TestComponentStart(t *testing.T) {
	t.Parallel()

	comp := &component{}
	err := comp.Start()
	assert.NoError(t, err)
}

func TestComponentStates(t *testing.T) {
	t.Parallel()

	comp := &component{}

	states := comp.LastHealthStates()
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.HealthStateTypeHealthy, states[0].Health)
	assert.Equal(t, "no issue", states[0].Reason)
}

func TestComponentEvents(t *testing.T) {
	t.Parallel()

	// Create mock event bucket
	mockEventBucket := new(MockEventBucket)
	testTime := metav1.Now()
	testEvents := apiv1.Events{
		{
			Time:                testTime,
			Name:                "test-nccl-error",
			Type:                "Warning",
			Message:             "This is a test NCCL error",
			DeprecatedExtraInfo: map[string]string{"log_line": "test-error-line"},
		},
	}

	mockEventBucket.On("Get", mock.Anything, mock.Anything).Return(testEvents, nil)
	mockEventBucket.On("Close").Return()

	comp := &component{
		eventBucket: mockEventBucket,
		// We don't set kmsgSyncer here since we don't need it for the test
	}

	// Call Events
	ctx := context.Background()
	since := time.Now().Add(-1 * time.Hour)
	events, err := comp.Events(ctx, since)

	// Verify results
	assert.NoError(t, err)
	require.NotNil(t, events)
	assert.Len(t, events, 1)
	assert.Equal(t, "test-nccl-error", events[0].Name)
	assert.Equal(t, "This is a test NCCL error", events[0].Message)

	mockEventBucket.AssertCalled(t, "Get", mock.Anything, since)
}

func TestComponentClose(t *testing.T) {
	t.Parallel()

	// Create mock event bucket
	mockEventBucket := new(MockEventBucket)
	mockEventBucket.On("Close").Return()

	comp := &component{
		eventBucket: mockEventBucket,
		// We don't set kmsgSyncer here since we'll handle it in our assertions
	}

	err := comp.Close()
	assert.NoError(t, err)

	mockEventBucket.AssertCalled(t, "Close")
}

func TestEventsWithNilBucket(t *testing.T) {
	t.Parallel()

	comp := &component{
		eventBucket: nil,
	}

	ctx := context.Background()
	events, err := comp.Events(ctx, time.Now().Add(-1*time.Hour))

	assert.NoError(t, err)
	assert.Nil(t, events)
}

// TestMatchFunctionality tests the Match function in the component
func TestMatchFunctionality(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		input       string
		wantEvent   string
		wantMessage string
	}{
		{
			name:        "no match",
			input:       "some random log line",
			wantEvent:   "",
			wantMessage: "",
		},
		{
			name:        "segfault in libnccl",
			input:       "kernel: process[12345]: segfault at 0x00 ip 0x00 sp 0x00 error 4 in libnccl.so.2",
			wantEvent:   "nvidia_nccl_segfault_in_libnccl",
			wantMessage: "",
		},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotEvent, gotMessage := Match(tc.input)
			assert.Equal(t, tc.wantEvent, gotEvent)
			// Only check message if we expect a match
			if tc.wantEvent != "" {
				assert.NotEmpty(t, gotMessage)
			}
		})
	}
}
