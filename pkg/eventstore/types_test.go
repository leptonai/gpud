package eventstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithEventNamesToSelect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		eventNames  []string
		expected    map[string]any
		description string
	}{
		{
			name:        "single event name",
			eventNames:  []string{"kmsg"},
			expected:    map[string]any{"kmsg": true},
			description: "should add single event name to select map",
		},
		{
			name:        "multiple event names",
			eventNames:  []string{"kmsg", "syslog", "nvidia"},
			expected:    map[string]any{"kmsg": true, "syslog": true, "nvidia": true},
			description: "should add multiple event names to select map",
		},
		{
			name:        "empty event names",
			eventNames:  []string{},
			expected:    map[string]any{},
			description: "should handle empty event names list with empty map",
		},
		{
			name:        "special characters in event names",
			eventNames:  []string{"event'with'quotes", "event;with;semicolon", "event,with,comma"},
			expected:    map[string]any{"event'with'quotes": true, "event;with;semicolon": true, "event,with,comma": true},
			description: "should handle event names with special characters",
		},
		{
			name:        "duplicate event names",
			eventNames:  []string{"kmsg", "kmsg", "syslog"},
			expected:    map[string]any{"kmsg": true, "syslog": true},
			description: "should handle duplicate event names (map should dedupe)",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			op := &Op{}
			optionFunc := WithEventNamesToSelect(tt.eventNames...)
			optionFunc(op)

			assert.Equal(t, tt.expected, op.eventNamesToSelect, tt.description)
		})
	}
}

func TestWithEventNamesToExclude(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		eventNames  []string
		expected    map[string]any
		description string
	}{
		{
			name:        "single event name",
			eventNames:  []string{"kmsg"},
			expected:    map[string]any{"kmsg": true},
			description: "should add single event name to exclude map",
		},
		{
			name:        "multiple event names",
			eventNames:  []string{"kmsg", "syslog", "nvidia"},
			expected:    map[string]any{"kmsg": true, "syslog": true, "nvidia": true},
			description: "should add multiple event names to exclude map",
		},
		{
			name:        "empty event names",
			eventNames:  []string{},
			expected:    map[string]any{},
			description: "should handle empty event names list with empty map",
		},
		{
			name:        "special characters in event names",
			eventNames:  []string{"event'with'quotes", "event;with;semicolon", "event,with,comma"},
			expected:    map[string]any{"event'with'quotes": true, "event;with;semicolon": true, "event,with,comma": true},
			description: "should handle event names with special characters",
		},
		{
			name:        "duplicate event names",
			eventNames:  []string{"kmsg", "kmsg", "syslog"},
			expected:    map[string]any{"kmsg": true, "syslog": true},
			description: "should handle duplicate event names (map should dedupe)",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			op := &Op{}
			optionFunc := WithEventNamesToExclude(tt.eventNames...)
			optionFunc(op)

			assert.Equal(t, tt.expected, op.eventNamesToExclude, tt.description)
		})
	}
}

func TestWithDisablePurge(t *testing.T) {
	t.Parallel()

	op := &Op{}
	optionFunc := WithDisablePurge()
	optionFunc(op)

	assert.True(t, op.disablePurge, "should set disablePurge to true")
}

func TestOpApplyOpts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		options     []OpOption
		expectedErr error
		description string
		validate    func(t *testing.T, op *Op)
	}{
		{
			name:        "no options",
			options:     []OpOption{},
			expectedErr: nil,
			description: "should handle no options without error",
			validate: func(t *testing.T, op *Op) {
				assert.False(t, op.disablePurge)
				assert.Nil(t, op.eventNamesToSelect)
				assert.Nil(t, op.eventNamesToExclude)
			},
		},
		{
			name:        "single option - disable purge",
			options:     []OpOption{WithDisablePurge()},
			expectedErr: nil,
			description: "should apply single option correctly",
			validate: func(t *testing.T, op *Op) {
				assert.True(t, op.disablePurge)
				assert.Nil(t, op.eventNamesToSelect)
				assert.Nil(t, op.eventNamesToExclude)
			},
		},
		{
			name:        "single option - select events",
			options:     []OpOption{WithEventNamesToSelect("kmsg", "syslog")},
			expectedErr: nil,
			description: "should apply select option correctly",
			validate: func(t *testing.T, op *Op) {
				assert.False(t, op.disablePurge)
				assert.Equal(t, map[string]any{"kmsg": true, "syslog": true}, op.eventNamesToSelect)
				assert.Nil(t, op.eventNamesToExclude)
			},
		},
		{
			name:        "single option - exclude events",
			options:     []OpOption{WithEventNamesToExclude("nvidia", "dmesg")},
			expectedErr: nil,
			description: "should apply exclude option correctly",
			validate: func(t *testing.T, op *Op) {
				assert.False(t, op.disablePurge)
				assert.Nil(t, op.eventNamesToSelect)
				assert.Equal(t, map[string]any{"nvidia": true, "dmesg": true}, op.eventNamesToExclude)
			},
		},
		{
			name: "multiple compatible options",
			options: []OpOption{
				WithDisablePurge(),
				WithEventNamesToSelect("kmsg"),
			},
			expectedErr: nil,
			description: "should apply multiple compatible options correctly",
			validate: func(t *testing.T, op *Op) {
				assert.True(t, op.disablePurge)
				assert.Equal(t, map[string]any{"kmsg": true}, op.eventNamesToSelect)
				assert.Nil(t, op.eventNamesToExclude)
			},
		},
		{
			name: "multiple compatible options with exclude",
			options: []OpOption{
				WithDisablePurge(),
				WithEventNamesToExclude("nvidia"),
			},
			expectedErr: nil,
			description: "should apply multiple compatible options with exclude correctly",
			validate: func(t *testing.T, op *Op) {
				assert.True(t, op.disablePurge)
				assert.Nil(t, op.eventNamesToSelect)
				assert.Equal(t, map[string]any{"nvidia": true}, op.eventNamesToExclude)
			},
		},
		{
			name: "conflicting options - select and exclude",
			options: []OpOption{
				WithEventNamesToSelect("kmsg"),
				WithEventNamesToExclude("nvidia"),
			},
			expectedErr: ErrEventNamesToSelectAndExclude,
			description: "should return error when both select and exclude are used",
			validate: func(t *testing.T, op *Op) {
				// Op state after error is not guaranteed, so we don't validate
			},
		},
		{
			name: "conflicting options - exclude and select",
			options: []OpOption{
				WithEventNamesToExclude("nvidia"),
				WithEventNamesToSelect("kmsg"),
			},
			expectedErr: ErrEventNamesToSelectAndExclude,
			description: "should return error regardless of order",
			validate: func(t *testing.T, op *Op) {
				// Op state after error is not guaranteed, so we don't validate
			},
		},
		{
			name: "multiple calls to same option type",
			options: []OpOption{
				WithEventNamesToSelect("kmsg"),
				WithEventNamesToSelect("syslog"),
			},
			expectedErr: nil,
			description: "should accumulate multiple calls to same option type",
			validate: func(t *testing.T, op *Op) {
				assert.False(t, op.disablePurge)
				assert.Equal(t, map[string]any{"kmsg": true, "syslog": true}, op.eventNamesToSelect)
				assert.Nil(t, op.eventNamesToExclude)
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			op := &Op{}
			err := op.applyOpts(tt.options)

			if tt.expectedErr != nil {
				assert.Equal(t, tt.expectedErr, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				tt.validate(t, op)
			}
		})
	}
}

func TestEventNamesToSelectAndExcludeError(t *testing.T) {
	t.Parallel()

	// Test the error constant
	assert.Equal(t, "cannot use both event names to select/exclude", ErrEventNamesToSelectAndExclude.Error())

	// Test error type
	assert.Implements(t, (*error)(nil), ErrEventNamesToSelectAndExclude)
}

func TestEventToEventConversion(t *testing.T) {
	t.Parallel()

	// This test is for the existing Event.ToEvent() method to ensure it works correctly
	// with the context of filtering functionality

	testEvent := Event{
		Component: "test-component",
		Name:      "test-event",
		Type:      "Warning",
		Message:   "Test message",
	}

	apiEvent := testEvent.ToEvent()

	assert.Equal(t, testEvent.Component, apiEvent.Component)
	assert.Equal(t, testEvent.Name, apiEvent.Name)
	assert.Equal(t, testEvent.Type, string(apiEvent.Type))
	assert.Equal(t, testEvent.Message, apiEvent.Message)
}

func TestEventsToEventsConversion(t *testing.T) {
	t.Parallel()

	// Test the Events.Events() method to ensure it works correctly
	testEvents := Events{
		{
			Component: "component1",
			Name:      "event1",
			Type:      "Info",
			Message:   "Message 1",
		},
		{
			Component: "component2",
			Name:      "event2",
			Type:      "Warning",
			Message:   "Message 2",
		},
	}

	apiEvents := testEvents.Events()

	assert.Equal(t, len(testEvents), len(apiEvents))
	for i, event := range testEvents {
		assert.Equal(t, event.Component, apiEvents[i].Component)
		assert.Equal(t, event.Name, apiEvents[i].Name)
		assert.Equal(t, event.Type, string(apiEvents[i].Type))
		assert.Equal(t, event.Message, apiEvents[i].Message)
	}
}