package common

import (
	"reflect"
	"testing"
)

func TestSimplify(t *testing.T) {
	t.Run("Empty repair actions", func(t *testing.T) {
		sa := &SuggestedActions{
			References:    []string{"ref1", "ref2"},
			Descriptions:  []string{"desc1", "desc2"},
			RepairActions: []RepairActionType{},
		}

		simplified := sa.Simplify()

		if len(simplified.RepairActions) != 0 {
			t.Errorf("Expected empty repair actions, got %v", simplified.RepairActions)
		}
		if simplified.References != nil {
			t.Errorf("Expected nil references, got %v", simplified.References)
		}
		if simplified.Descriptions != nil {
			t.Errorf("Expected nil descriptions, got %v", simplified.Descriptions)
		}
	})

	t.Run("Unique repair actions", func(t *testing.T) {
		sa := &SuggestedActions{
			References:   []string{"ref1", "ref2"},
			Descriptions: []string{"desc1", "desc2"},
			RepairActions: []RepairActionType{
				RepairActionTypeIgnoreNoActionRequired,
				RepairActionTypeCheckUserAppAndGPU,
			},
		}

		simplified := sa.Simplify()

		expected := []RepairActionType{
			RepairActionTypeIgnoreNoActionRequired,
			RepairActionTypeCheckUserAppAndGPU,
		}

		if !hasSameActions(simplified.RepairActions, expected) {
			t.Errorf("Expected %v, got %v", expected, simplified.RepairActions)
		}
	})

	t.Run("Duplicate repair actions", func(t *testing.T) {
		sa := &SuggestedActions{
			RepairActions: []RepairActionType{
				RepairActionTypeIgnoreNoActionRequired,
				RepairActionTypeIgnoreNoActionRequired,
				RepairActionTypeCheckUserAppAndGPU,
			},
		}

		simplified := sa.Simplify()

		expected := []RepairActionType{
			RepairActionTypeIgnoreNoActionRequired,
			RepairActionTypeCheckUserAppAndGPU,
		}

		if !hasSameActions(simplified.RepairActions, expected) {
			t.Errorf("Expected %v, got %v", expected, simplified.RepairActions)
		}
	})

	t.Run("Hardware inspection removes reboot system", func(t *testing.T) {
		sa := &SuggestedActions{
			RepairActions: []RepairActionType{
				RepairActionTypeHardwareInspection,
				RepairActionTypeRebootSystem,
				RepairActionTypeCheckUserAppAndGPU,
			},
		}

		simplified := sa.Simplify()

		expected := []RepairActionType{
			RepairActionTypeHardwareInspection,
			RepairActionTypeCheckUserAppAndGPU,
		}

		if !hasSameActions(simplified.RepairActions, expected) {
			t.Errorf("Expected %v, got %v", expected, simplified.RepairActions)
		}

		// Ensure RebootSystem is not in the simplified actions
		for _, action := range simplified.RepairActions {
			if action == RepairActionTypeRebootSystem {
				t.Errorf("RebootSystem should have been removed, but was found in %v", simplified.RepairActions)
			}
		}
	})

	t.Run("Keep reboot system when hardware inspection is not present", func(t *testing.T) {
		sa := &SuggestedActions{
			RepairActions: []RepairActionType{
				RepairActionTypeRebootSystem,
				RepairActionTypeCheckUserAppAndGPU,
			},
		}

		simplified := sa.Simplify()

		expected := []RepairActionType{
			RepairActionTypeRebootSystem,
			RepairActionTypeCheckUserAppAndGPU,
		}

		if !hasSameActions(simplified.RepairActions, expected) {
			t.Errorf("Expected %v, got %v", expected, simplified.RepairActions)
		}
	})
}

// hasSameActions checks if two slices of RepairActionType contain the same actions
// regardless of their order
func hasSameActions(a, b []RepairActionType) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[RepairActionType]int)
	for _, action := range a {
		aMap[action]++
	}

	bMap := make(map[RepairActionType]int)
	for _, action := range b {
		bMap[action]++
	}

	return reflect.DeepEqual(aMap, bMap)
}
