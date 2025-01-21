package sqlite

import (
	"math"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("failed to register metrics: %v", err)
	}

	mtr, err := ReadMetrics(reg)
	if err != nil {
		t.Fatalf("failed to read select metrics: %v", err)
	}
	if mtr.InsertUpdateTotal != 0 {
		t.Fatalf("initial insert/update total should be 0, got %d", mtr.InsertUpdateTotal)
	}
	if mtr.InsertUpdateSecondsTotal != 0 {
		t.Fatalf("initial insert/update seconds total should be 0, got %f", mtr.InsertUpdateSecondsTotal)
	}
	if mtr.DeleteTotal != 0 {
		t.Fatalf("initial delete total should be 0, got %d", mtr.DeleteTotal)
	}
	if mtr.DeleteSecondsTotal != 0 {
		t.Fatalf("initial delete seconds total should be 0, got %f", mtr.DeleteSecondsTotal)
	}
	if mtr.SelectTotal != 0 {
		t.Fatalf("initial select total should be 0, got %d", mtr.SelectTotal)
	}
	if mtr.SelectSecondsTotal != 0 {
		t.Fatalf("initial select seconds total should be 0, got %f", mtr.SelectSecondsTotal)
	}
	if mtr.SelectSecondsAvg != 0 {
		t.Fatalf("initial select seconds avg should be 0, got %f", mtr.SelectSecondsAvg)
	}

	const (
		inserts          = 10
		secondsPerInsert = float64(0.7)
	)
	expectedSecondsWrites := float64(inserts) * secondsPerInsert

	for i := 0; i < inserts; i++ {
		RecordInsertUpdate(secondsPerInsert)
	}

	mtr, err = ReadMetrics(reg)
	if err != nil {
		t.Fatalf("failed to read insert/update metrics: %v", err)
	}
	if mtr.InsertUpdateTotal != int64(inserts) {
		t.Fatalf("expected %d inserts, got %d", inserts, mtr.InsertUpdateTotal)
	}
	if !floatEquals(mtr.InsertUpdateSecondsTotal, expectedSecondsWrites) {
		t.Fatalf("expected %.3f seconds total for inserts, got %.3f", expectedSecondsWrites, mtr.InsertUpdateSecondsTotal)
	}
	if !floatEquals(mtr.InsertUpdateSecondsAvg, secondsPerInsert) {
		t.Fatalf("expected %.3f seconds avg for inserts, got %.3f", secondsPerInsert, mtr.InsertUpdateSecondsAvg)
	}

	const (
		deletes          = 5
		secondsPerDelete = float64(0.9)
	)
	expectedSecondsDeletes := float64(deletes) * secondsPerDelete
	for i := 0; i < deletes; i++ {
		RecordDelete(secondsPerDelete)
	}

	mtr, err = ReadMetrics(reg)
	if err != nil {
		t.Fatalf("failed to read delete metrics: %v", err)
	}
	if mtr.DeleteTotal != int64(deletes) {
		t.Fatalf("expected %d deletes, got %d", deletes, mtr.DeleteTotal)
	}
	if !floatEquals(mtr.DeleteSecondsTotal, expectedSecondsDeletes) {
		t.Fatalf("expected %.3f seconds total for deletes, got %.3f", expectedSecondsDeletes, mtr.DeleteSecondsTotal)
	}
	if !floatEquals(mtr.DeleteSecondsAvg, secondsPerDelete) {
		t.Fatalf("expected %.3f seconds avg for deletes, got %.3f", secondsPerDelete, mtr.DeleteSecondsAvg)
	}

	const (
		selects       = 20
		secsPerSelect = 0.50
	)
	expectedSecondsSelect := float64(selects) * secsPerSelect

	for i := 0; i < selects; i++ {
		RecordSelect(secsPerSelect)
	}

	mtr, err = ReadMetrics(reg)
	if err != nil {
		t.Fatalf("failed to read select metrics: %v", err)
	}
	if mtr.SelectTotal != int64(selects) {
		t.Fatalf("expected %d selects, got %d", selects, mtr.SelectTotal)
	}
	if !floatEquals(mtr.SelectSecondsTotal, expectedSecondsSelect) {
		t.Fatalf("expected %.3f seconds total for selects, got %.3f", expectedSecondsSelect, mtr.SelectSecondsTotal)
	}
	if mtr.InsertUpdateTotal != int64(inserts) {
		t.Fatalf("insert count changed unexpectedly: expected %d, got %d", inserts, mtr.InsertUpdateTotal)
	}
	if !floatEquals(mtr.InsertUpdateSecondsTotal, expectedSecondsWrites) {
		t.Fatalf("insert seconds changed unexpectedly: expected %.3f, got %.3f", expectedSecondsWrites, mtr.InsertUpdateSecondsTotal)
	}
	if !floatEquals(mtr.DeleteSecondsAvg, secondsPerDelete) {
		t.Fatalf("delete seconds avg changed unexpectedly: expected %.3f, got %.3f", secondsPerDelete, mtr.DeleteSecondsAvg)
	}
	if !floatEquals(mtr.SelectSecondsAvg, secsPerSelect) {
		t.Fatalf("select seconds avg changed unexpectedly: expected %.3f, got %.3f", secsPerSelect, mtr.SelectSecondsAvg)
	}
}

func floatEquals(a, b float64) bool {
	return math.Abs(a-b) < 0.0005
}
