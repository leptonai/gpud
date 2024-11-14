package query

import (
	"context"
	"reflect"
	"testing"
	"time"

	query_config "github.com/leptonai/gpud/components/query/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPoller_ReadLastItemFromInMemoryQueue(t *testing.T) {
	pl := &poller{
		lastItems: []Item{},
	}

	// Test empty queue
	item, err := pl.readLastItemFromInMemoryQueue()
	if err != ErrNoData {
		t.Errorf("expected ErrNoData, got %v", err)
	}
	if item != nil {
		t.Errorf("expected nil item, got %v", item)
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

func TestPoller_processResult(t *testing.T) {
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
			name:              "Add to empty queue",
			queueN:            3,
			initial:           []Item{},
			newResult:         &Item{Time: metav1.NewTime(now)},
			expectedLastItems: []Item{{Time: metav1.NewTime(now)}},
			expectedLastErr:   nil,
		},
		{
			name:   "Add to non-full queue",
			queueN: 3,
			initial: []Item{
				{Time: metav1.NewTime(now.Add(-2 * time.Second))},
			},
			newResult: &Item{Time: metav1.NewTime(now)},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now.Add(-2 * time.Second))},
				{Time: metav1.NewTime(now)},
			},
			expectedLastErr: nil,
		},
		{
			name:   "Add to full queue",
			queueN: 3,
			initial: []Item{
				{Time: metav1.NewTime(now.Add(-3 * time.Second))},
				{Time: metav1.NewTime(now.Add(-2 * time.Second))},
				{Time: metav1.NewTime(now.Add(-1 * time.Second))},
			},
			newResult: &Item{Time: metav1.NewTime(now)},
			expectedLastItems: []Item{
				{Time: metav1.NewTime(now.Add(-2 * time.Second))},
				{Time: metav1.NewTime(now.Add(-1 * time.Second))},
				{Time: metav1.NewTime(now)},
			},
			expectedLastErr: nil,
		},
		{
			name:              "Empty queue",
			queueN:            3,
			initial:           []Item{},
			newResult:         nil,
			expectedLastItems: []Item{},
			expectedLastErr:   ErrNoData,
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
			if !reflect.DeepEqual(tt.expectedLastItems, q.lastItems) {
				t.Errorf("expected %+v, got %+v", tt.expectedLastItems, q.lastItems)
			}
			if len(q.lastItems) > tt.queueN {
				t.Errorf("expected queue length of %d, got %d", tt.queueN, len(q.lastItems))
			}
			last, err := q.Last()
			if err != tt.expectedLastErr {
				t.Errorf("expected last error %v, got %v", tt.expectedLastErr, err)
			}
			if err == nil && !reflect.DeepEqual(last, &q.lastItems[len(q.lastItems)-1]) {
				t.Errorf("expected last item %+v, got %+v", q.lastItems[len(q.lastItems)-1], last)
			}
		})
	}
}

func TestPollerStartStop(t *testing.T) {
	startFuncCalled := 0
	cancelCalled := 0
	q := &poller{
		startPollFunc: func(ctx context.Context, id string, interval time.Duration, _ GetFunc) <-chan Item {
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
