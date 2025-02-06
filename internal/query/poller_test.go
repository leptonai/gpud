package query

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	query_config "github.com/leptonai/gpud/internal/query/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPollerReadLast(t *testing.T) {
	now := time.Now()
	pl := &poller{
		lastItems: []Item{
			{Time: metav1.NewTime(now.Add(-1 * time.Second))},
			{Time: metav1.NewTime(now)},
		},
	}

	item, err := pl.readLast(true)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !reflect.DeepEqual(item, &pl.lastItems[len(pl.lastItems)-1]) {
		t.Errorf("expected last item %+v, got %+v", pl.lastItems[len(pl.lastItems)-1], item)
	}
}

func TestPollerReadLastWithErr(t *testing.T) {
	pl := &poller{
		lastItems: []Item{
			{Time: metav1.NewTime(time.Unix(1, 0))},
			{Time: metav1.NewTime(time.Unix(2, 0)), Error: errors.New("test error")},
			{Time: metav1.NewTime(time.Unix(3, 0)), Error: errors.New("test error")},
		},
	}

	item, err := pl.readLast(true)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !reflect.DeepEqual(item, &pl.lastItems[0]) {
		t.Errorf("expected last item %+v, got %+v", pl.lastItems[0], item)
	}
}

func TestPollerReadLastNoData(t *testing.T) {
	pl := &poller{
		lastItems: []Item{
			{Time: metav1.NewTime(time.Unix(1, 0)), Error: errors.New("test error")},
			{Time: metav1.NewTime(time.Unix(2, 0)), Error: errors.New("test error")},
			{Time: metav1.NewTime(time.Unix(3, 0)), Error: errors.New("test error")},
		},
	}

	item, err := pl.readLast(true)
	if item != nil {
		t.Errorf("expected nil item, got %+v", item)
	}
	if !errors.Is(err, ErrNoData) {
		t.Errorf("expected ErrNoData, got %v", err)
	}
}

func TestPoller_ReadAllItemsFromInMemoryQueue(t *testing.T) {
	pl := &poller{
		lastItems: []Item{},
	}

	// Test empty queue
	items, err := pl.readAllItemsFromInMemoryQueue(time.Time{})
	if err != ErrNoData {
		t.Errorf("expected ErrNoData, got %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items, got %v", items)
	}

	// Test with since time but empty queue
	since := time.Now()
	items, err = pl.readAllItemsFromInMemoryQueue(since)
	if err != ErrNoData {
		t.Errorf("expected ErrNoData, got %v", err)
	}
	if items != nil {
		t.Errorf("expected nil items, got %v", items)
	}
}

func TestPoller_processItemExtended(t *testing.T) {
	now := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tests := []struct {
		name              string
		queueN            int
		initial           []Item
		newResult         *Item
		expectedLastItems []Item
		expectedLastErr   error
	}{
		{
			name:   "Add item with error",
			queueN: 3,
			initial: []Item{
				{Time: metav1.NewTime(now.Add(-2 * time.Second))},
			},
			newResult: &Item{
				Time:  metav1.NewTime(now),
				Error: errors.New("test error"),
			},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now.Add(-2 * time.Second))},
				{Time: metav1.NewTime(now), Error: errors.New("test error")},
			},
			expectedLastErr: nil,
		},
		{
			name:   "Add multiple items with errors",
			queueN: 3,
			initial: []Item{
				{Time: metav1.NewTime(now.Add(-3 * time.Second))},
			},
			newResult: &Item{
				Time:  metav1.NewTime(now),
				Error: errors.New("another error"),
			},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now.Add(-3 * time.Second))},
				{Time: metav1.NewTime(now), Error: errors.New("another error")},
			},
			expectedLastErr: nil,
		},
		{
			name:   "Queue at capacity with new error item",
			queueN: 2,
			initial: []Item{
				{Time: metav1.NewTime(now.Add(-2 * time.Second))},
				{Time: metav1.NewTime(now.Add(-1 * time.Second))},
			},
			newResult: &Item{
				Time:  metav1.NewTime(now),
				Error: errors.New("overflow error"),
			},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now.Add(-1 * time.Second))},
				{Time: metav1.NewTime(now), Error: errors.New("overflow error")},
			},
			expectedLastErr: nil,
		},
		{
			name:   "Add item with older timestamp",
			queueN: 3,
			initial: []Item{
				{Time: metav1.NewTime(now)},
			},
			newResult: &Item{
				Time: metav1.NewTime(now.Add(-5 * time.Second)),
			},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now)},
				{Time: metav1.NewTime(now.Add(-5 * time.Second))},
			},
			expectedLastErr: nil,
		},
		{
			name:   "Add duplicate timestamp",
			queueN: 3,
			initial: []Item{
				{Time: metav1.NewTime(now)},
			},
			newResult: &Item{
				Time: metav1.NewTime(now),
			},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now)},
				{Time: metav1.NewTime(now)},
			},
			expectedLastErr: nil,
		},
		{
			name:   "Queue exactly at capacity",
			queueN: 1,
			initial: []Item{
				{Time: metav1.NewTime(now.Add(-1 * time.Second))},
			},
			newResult: &Item{
				Time: metav1.NewTime(now),
			},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now)},
			},
			expectedLastErr: nil,
		},
		{
			name:   "Add nil error item",
			queueN: 3,
			initial: []Item{
				{Time: metav1.NewTime(now.Add(-1 * time.Second)), Error: errors.New("previous error")},
			},
			newResult: &Item{
				Time:  metav1.NewTime(now),
				Error: nil,
			},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now.Add(-1 * time.Second)), Error: errors.New("previous error")},
				{Time: metav1.NewTime(now), Error: nil},
			},
			expectedLastErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &poller{
				ctx:       ctx,
				cfg:       query_config.Config{QueueSize: tt.queueN},
				lastItems: tt.initial,
			}
			if tt.newResult != nil {
				q.processItem(*tt.newResult)
			}

			// Check queue size constraint
			if len(q.lastItems) > tt.queueN && tt.queueN > 0 {
				t.Errorf("queue size exceeded: got %d, want <= %d", len(q.lastItems), tt.queueN)
			}

			// Check items length
			if len(q.lastItems) != len(tt.expectedLastItems) {
				t.Errorf("unexpected number of items: got %d, want %d", len(q.lastItems), len(tt.expectedLastItems))
			}

			// Check each item
			for i := range tt.expectedLastItems {
				if !tt.expectedLastItems[i].Time.Time.Equal(q.lastItems[i].Time.Time) {
					t.Errorf("item %d: unexpected time: got %v, want %v", i, q.lastItems[i].Time, tt.expectedLastItems[i].Time)
				}

				// Compare error values considering nil cases
				if (tt.expectedLastItems[i].Error == nil) != (q.lastItems[i].Error == nil) {
					t.Errorf("item %d: unexpected error state: got %v, want %v", i, q.lastItems[i].Error, tt.expectedLastItems[i].Error)
				} else if tt.expectedLastItems[i].Error != nil && q.lastItems[i].Error != nil && tt.expectedLastItems[i].Error.Error() != q.lastItems[i].Error.Error() {
					t.Errorf("item %d: unexpected error message: got %v, want %v", i, q.lastItems[i].Error, tt.expectedLastItems[i].Error)
				}
			}

			// Check last item retrieval
			last, err := q.Last()
			if err != tt.expectedLastErr {
				t.Errorf("unexpected Last() error: got %v, want %v", err, tt.expectedLastErr)
			}
			if err == nil && !reflect.DeepEqual(last, &q.lastItems[len(q.lastItems)-1]) {
				t.Errorf("unexpected last item: got %+v, want %+v", last, q.lastItems[len(q.lastItems)-1])
			}
		})
	}
}

func TestPollerStartStop(t *testing.T) {
	startFuncCalled := 0
	cancelCalled := 0
	q := &poller{
		startPollFunc: func(ctx context.Context, id string, interval time.Duration, _ time.Duration, _ GetFunc, _ GetErrHandler) <-chan Item {
			t.Log("startFunc called")
			startFuncCalled++
			return make(<-chan Item)
		},

		cfg:       query_config.Config{QueueSize: 3},
		lastItems: []Item{},

		inflightComponents: make(map[string]any),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q.Start(ctx, query_config.Config{Interval: metav1.Duration{Duration: time.Second}}, "test1")
	q.Start(ctx, query_config.Config{Interval: metav1.Duration{Duration: time.Second}}, "test2")
	q.Start(ctx, query_config.Config{Interval: metav1.Duration{Duration: time.Second}}, "test3")

	q.cancel = context.CancelFunc(func() {
		t.Log("cancel called")
		cancelCalled++
	})

	canceled := q.Stop("test1")
	if canceled {
		t.Errorf("expected cancel to be called, got true")
	}
	// do not cancel if there's an inflight
	if cancelCalled != 0 {
		t.Errorf("expected cancel to be called 0 time, got %d", cancelCalled)
	}

	canceled = q.Stop("test2")
	if canceled {
		t.Errorf("expected cancel to be called, got true")
	}
	// do not cancel if there's an inflight
	if cancelCalled != 0 {
		t.Errorf("expected cancel to be called 0 time, got %d", cancelCalled)
	}

	// no inflight, so should be calling "cancel"
	canceled = q.Stop("test3")
	if !canceled {
		t.Errorf("expected cancel to be called, got false")
	}

	// no inflight, so should be calling "cancel"
	if cancelCalled != 1 {
		t.Errorf("expected cancel to be called 1 time, got %d", cancelCalled)
	}

	// no-op duplicate calls
	canceled = q.Stop("test1")
	if canceled {
		t.Errorf("expected cancel to be called, got true")
	}

	canceled = q.Stop("test2")
	if canceled {
		t.Errorf("expected cancel to be called, got true")
	}

	canceled = q.Stop("test3")
	if canceled {
		t.Errorf("expected cancel to be called, got true")
	}

	// no inflight, so should be calling "cancel"
	if cancelCalled != 1 {
		t.Errorf("expected cancel to be called 1 time, got %d", cancelCalled)
	}

	// no redundant start calls
	if startFuncCalled != 1 {
		t.Errorf("expected startFunc to be called 1 time, got %d", startFuncCalled)
	}
}

// Return nil when no errors found in lastItems array
func TestReadLastErrReturnsNilWhenNoErrors(t *testing.T) {
	pl := &poller{
		lastItems: []Item{
			{Error: nil},
			{Error: nil},
			{Error: nil},
		},
	}

	err := pl.readLastErr()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// Handle empty lastItems array returning ErrNoData
func TestReadLastErrReturnsErrNoDataForEmptyArray(t *testing.T) {
	pl := &poller{
		lastItems: []Item{},
	}

	err := pl.readLastErr()
	if !errors.Is(err, ErrNoData) {
		t.Errorf("expected ErrNoData, got %v", err)
	}
}
