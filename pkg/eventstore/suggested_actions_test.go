package eventstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestEvaluateSuggestedActions(t *testing.T) {
	now := time.Now()

	t.Run("case 1 - no reboot events", func(t *testing.T) {
		rebootEvents := Events{}
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now,
				Name:      "test-failure",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Test failure",
			},
		}

		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 2)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, actions.RepairActions[0])
	})

	t.Run("case 2 - one reboot before failure", func(t *testing.T) {
		rebootEvents := Events{
			{
				Time:    now.Add(-2 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now,
				Name:      "test-failure",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Test failure",
			},
		}

		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 2)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, actions.RepairActions[0])
	})

	t.Run("case 3 - single reboot single failure (should suggest reboot)", func(t *testing.T) {
		// With only 1 reboot and 1 failure, should suggest reboot according to the logic
		// Timeline: First reboot -> First failure
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now.Add(-2 * time.Hour), // Single failure (after reboot)
				Name:      "test-failure-1",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "First failure",
			},
		}
		rebootEvents := Events{
			{
				Time:    now.Add(-4 * time.Hour), // Single reboot before failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}

		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 2)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		// With only 1 reboot or 1 failure, should suggest reboot
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, actions.RepairActions[0])
	})

	t.Run("case 4 - two failure-to-reboot sequences (should suggest HW inspection)", func(t *testing.T) {
		// Create proper timeline for 2 failure->reboot sequences:
		// Reboot1 -> Failure1 -> Reboot2 -> Failure2 -> Failure3
		// Failure1 and Failure2 both happened after reboots, creating 2 sequences
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now.Add(-5 * time.Hour), // First failure (after first reboot)
				Name:      "test-failure-1",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "First failure",
			},
			{
				Component: "test-component",
				Time:      now.Add(-2 * time.Hour), // Second failure (after second reboot)
				Name:      "test-failure-2",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Second failure",
			},
		}
		rebootEvents := Events{
			{
				Time:    now.Add(-6 * time.Hour), // First reboot before any failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
			{
				Time:    now.Add(-3 * time.Hour), // Second reboot after first failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}

		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 2)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		// With 2 failure->reboot sequences and threshold of 2, should suggest HW inspection
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, actions.RepairActions[0])
	})

	t.Run("reboot after failure - should return nil (edge case)", func(t *testing.T) {
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now.Add(-1 * time.Hour), // Failure happened first
				Name:      "test-failure",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Test failure",
			},
		}
		rebootEvents := Events{
			{
				Time:    now.Add(-30 * time.Minute), // Reboot happened after failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}

		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 2)

		// The logic returns nil if first reboot happened after first failure (edge case)
		assert.Nil(t, actions)
	})

	t.Run("custom reboot threshold - 3 sequences", func(t *testing.T) {
		// Create timeline with 3 failure->reboot sequences to test custom threshold
		// Timeline: Reboot1 -> Failure1 -> Reboot2 -> Failure2 -> Reboot3 -> Failure3
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now.Add(-7 * time.Hour), // First failure (after first reboot)
				Name:      "test-failure-1",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "First failure",
			},
			{
				Component: "test-component",
				Time:      now.Add(-5 * time.Hour), // Second failure (after second reboot)
				Name:      "test-failure-2",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Second failure",
			},
			{
				Component: "test-component",
				Time:      now.Add(-2 * time.Hour), // Third failure (after third reboot)
				Name:      "test-failure-3",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Third failure",
			},
		}
		rebootEvents := Events{
			{
				Time:    now.Add(-8 * time.Hour), // First reboot before any failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
			{
				Time:    now.Add(-6 * time.Hour), // Second reboot after first failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
			{
				Time:    now.Add(-3 * time.Hour), // Third reboot after second failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}

		// With threshold of 3, should suggest HW inspection after 3 sequences
		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 3)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, actions.RepairActions[0])
	})

	t.Run("custom reboot threshold - below threshold", func(t *testing.T) {
		// Only 2 sequences with threshold of 3
		// Timeline: Reboot1 -> Failure1 -> Reboot2 -> Failure2
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now.Add(-5 * time.Hour), // First failure (after first reboot)
				Name:      "test-failure-1",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "First failure",
			},
			{
				Component: "test-component",
				Time:      now.Add(-2 * time.Hour), // Second failure (after second reboot)
				Name:      "test-failure-2",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Second failure",
			},
		}
		rebootEvents := Events{
			{
				Time:    now.Add(-6 * time.Hour), // First reboot before any failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
			{
				Time:    now.Add(-3 * time.Hour), // Second reboot after first failure
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}

		// With threshold of 3, should still suggest reboot after only 2 sequences
		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 3)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, actions.RepairActions[0])
	})

	t.Run("invalid reboot threshold", func(t *testing.T) {
		rebootEvents := Events{}
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now,
				Name:      "test-failure",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Test failure",
			},
		}

		// Invalid threshold (0) should default to 2
		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 0)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, actions.RepairActions[0])
	})

	t.Run("multiple failure events with single reboot - edge case", func(t *testing.T) {
		// This test hits the edge case where failureEvents[0] (first in slice) happened before reboot,
		// even though there are subsequent failures after the reboot.
		// The current implementation returns nil because it only checks failureEvents[0] against first reboot.
		rebootEvents := Events{
			{
				Time:    now.Add(-2 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now.Add(-3 * time.Hour), // Older failure (before reboot) - this is failureEvents[0]
				Name:      "test-failure-1",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "First failure",
			},
			{
				Component: "test-component",
				Time:      now.Add(-1 * time.Hour), // Failure after reboot
				Name:      "test-failure-2",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Second failure",
			},
			{
				Component: "test-component",
				Time:      now.Add(-30 * time.Minute), // Another failure after reboot
				Name:      "test-failure-3",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Third failure",
			},
		}

		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 2)

		// Edge case: returns nil because failureEvents[0] happened before reboot,
		// even though there are failures after the reboot
		assert.Nil(t, actions)
	})

	t.Run("multiple failure events with single reboot - should suggest reboot", func(t *testing.T) {
		// Failures happen after reboot, so should suggest another reboot
		rebootEvents := Events{
			{
				Time:    now.Add(-3 * time.Hour), // Reboot before all failures
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now.Add(-2 * time.Hour), // Failure after reboot
				Name:      "test-failure-1",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "First failure",
			},
			{
				Component: "test-component",
				Time:      now.Add(-1 * time.Hour), // Another failure after reboot
				Name:      "test-failure-2",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Second failure",
			},
		}

		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 2)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		// Single reboot with failures after it, so should suggest another reboot
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, actions.RepairActions[0])
	})

	t.Run("no failure events", func(t *testing.T) {
		rebootEvents := Events{
			{
				Time:    now.Add(-2 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}
		failureEvents := Events{} // Empty

		// The implementation panics if no failure events are provided
		assert.Panics(t, func() {
			EvaluateSuggestedActions(rebootEvents, failureEvents, 2)
		})
	})

	t.Run("edge case - failures that map to same reboot", func(t *testing.T) {
		// Multiple failures after single reboot should only count as 1 sequence
		rebootEvents := Events{
			{
				Time:    now.Add(-5 * time.Hour),
				Name:    "reboot",
				Type:    string(apiv1.EventTypeWarning),
				Message: "system reboot",
			},
		}
		failureEvents := Events{
			{
				Component: "test-component",
				Time:      now.Add(-4 * time.Hour), // After reboot
				Name:      "test-failure-1",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "First failure",
			},
			{
				Component: "test-component",
				Time:      now.Add(-3 * time.Hour), // Also after same reboot
				Name:      "test-failure-2",
				Type:      string(apiv1.EventTypeWarning),
				Message:   "Second failure",
			},
		}

		actions := EvaluateSuggestedActions(rebootEvents, failureEvents, 2)

		assert.NotNil(t, actions)
		assert.Len(t, actions.RepairActions, 1)
		// Only 1 reboot, so should suggest another reboot despite multiple failures
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, actions.RepairActions[0])
	})
}

func TestAggregateSuggestedActions(t *testing.T) {
	t.Run("empty actions", func(t *testing.T) {
		result := AggregateSuggestedActions([]*apiv1.SuggestedActions{})
		assert.Nil(t, result)
	})

	t.Run("single reboot action", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}
		result := AggregateSuggestedActions(actions)
		assert.NotNil(t, result)
		assert.Len(t, result.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, result.RepairActions[0])
	})

	t.Run("multiple reboot actions", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}
		result := AggregateSuggestedActions(actions)
		assert.NotNil(t, result)
		assert.Len(t, result.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, result.RepairActions[0])
	})

	t.Run("HW_INSPECTION overrides REBOOT", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeHardwareInspection,
				},
			},
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
		}
		result := AggregateSuggestedActions(actions)
		assert.NotNil(t, result)
		assert.Len(t, result.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeHardwareInspection, result.RepairActions[0])
	})

	t.Run("nil actions in list", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			nil,
			{
				RepairActions: []apiv1.RepairActionType{
					apiv1.RepairActionTypeRebootSystem,
				},
			},
			nil,
		}
		result := AggregateSuggestedActions(actions)
		assert.NotNil(t, result)
		assert.Len(t, result.RepairActions, 1)
		assert.Equal(t, apiv1.RepairActionTypeRebootSystem, result.RepairActions[0])
	})

	t.Run("empty repair actions", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			{
				RepairActions: []apiv1.RepairActionType{},
			},
			{
				RepairActions: []apiv1.RepairActionType{},
			},
		}
		result := AggregateSuggestedActions(actions)
		assert.Nil(t, result)
	})

	t.Run("mixed nil and empty", func(t *testing.T) {
		actions := []*apiv1.SuggestedActions{
			nil,
			{
				RepairActions: []apiv1.RepairActionType{},
			},
			nil,
		}
		result := AggregateSuggestedActions(actions)
		assert.Nil(t, result)
	})
}
