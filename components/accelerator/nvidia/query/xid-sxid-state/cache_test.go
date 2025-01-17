package xidsxidstate

import (
	"testing"
	"time"
)

func TestEventDeduperExpire(t *testing.T) {
	deduper := NewEventDeduper(100*1024*1024, 3) // 3 seconds

	now := time.Now().Unix()
	ev := Event{
		UnixSeconds:  now,
		DataSource:   "dmesg",
		EventType:    "xid",
		EventID:      13,
		DeviceID:     "00000000-0000-0000-0000-000000000000",
		EventDetails: "test",
	}

	if err := deduper.Add(ev); err != nil {
		t.Fatalf("failed to add event to deduper: %v", err)
	}

	if !deduper.Get(ev) {
		t.Fatalf("event not found in deduper")
	}

	time.Sleep(5 * time.Second)

	if deduper.Get(ev) {
		t.Fatalf("event found in deduper after ttl")
	}
}

func TestEventDeduperByMinute(t *testing.T) {
	deduper := NewEventDeduper(100*1024*1024, 30)

	// Use a fixed timestamp at minute boundary (zero seconds)
	now := time.Date(2024, 1, 1, 10, 30, 0, 0, time.UTC).Unix()

	ev1 := Event{
		UnixSeconds:  now,
		DataSource:   "dmesg",
		EventType:    "xid",
		EventID:      13,
		DeviceID:     "00000000-0000-0000-0000-000000000000",
		EventDetails: "test",
	}
	ev2 := Event{
		UnixSeconds:  now + 30, // same minute but 30 seconds later
		DataSource:   "dmesg",
		EventType:    "xid",
		EventID:      13,
		DeviceID:     "00000000-0000-0000-0000-000000000000",
		EventDetails: "test",
	}

	if err := deduper.Add(ev1); err != nil {
		t.Fatalf("failed to add event to deduper: %v", err)
	}
	if !deduper.Get(ev1) {
		t.Fatalf("event not found in deduper")
	}
	// same minute thus, should be found
	if !deduper.Get(ev2) {
		t.Fatalf("event not found in deduper")
	}

	// redundant inserts are ok
	if err := deduper.Add(ev2); err != nil {
		t.Fatalf("failed to add event to deduper: %v", err)
	}
	if !deduper.Get(ev1) {
		t.Fatalf("event not found in deduper")
	}
	if !deduper.Get(ev2) {
		t.Fatalf("event not found in deduper")
	}
}
