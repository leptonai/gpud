package nccl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/eventstore"
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
	ctx := context.Background()

	states, err := comp.HealthStates(ctx)
	assert.NoError(t, err)
	assert.Len(t, states, 1)
	assert.Equal(t, apiv1.StateTypeHealthy, states[0].Health)
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
