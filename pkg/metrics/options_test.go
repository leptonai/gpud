package metrics

import (
	"reflect"
	"testing"
	"time"
)

func TestWithSince(t *testing.T) {
	testTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	op := &Op{}

	WithSince(testTime)(op)

	if !op.Since.Equal(testTime) {
		t.Errorf("WithSince() = %v, want %v", op.Since, testTime)
	}
}

func TestWithComponents(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		want       map[string]struct{}
	}{
		{
			name:       "single component",
			components: []string{"component1"},
			want:       map[string]struct{}{"component1": {}},
		},
		{
			name:       "multiple components",
			components: []string{"component1", "component2"},
			want:       map[string]struct{}{"component1": {}, "component2": {}},
		},
		{
			name:       "empty component is ignored",
			components: []string{"component1", ""},
			want:       map[string]struct{}{"component1": {}},
		},
		{
			name:       "all empty components",
			components: []string{"", ""},
			want:       map[string]struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			WithComponents(tt.components...)(op)

			if !reflect.DeepEqual(op.SelectedComponents, tt.want) {
				t.Errorf("WithComponents() = %v, want %v", op.SelectedComponents, tt.want)
			}
		})
	}
}

func TestApplyOpts(t *testing.T) {
	testTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		opts []OpOption
		want *Op
	}{
		{
			name: "no options",
			opts: []OpOption{},
			want: &Op{},
		},
		{
			name: "with since",
			opts: []OpOption{WithSince(testTime)},
			want: &Op{Since: testTime},
		},
		{
			name: "with components",
			opts: []OpOption{WithComponents("component1", "component2")},
			want: &Op{SelectedComponents: map[string]struct{}{"component1": {}, "component2": {}}},
		},
		{
			name: "with both options",
			opts: []OpOption{
				WithSince(testTime),
				WithComponents("component1", "component2"),
			},
			want: &Op{
				Since:              testTime,
				SelectedComponents: map[string]struct{}{"component1": {}, "component2": {}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &Op{}
			err := op.ApplyOpts(tt.opts)

			if err != nil {
				t.Errorf("ApplyOpts() error = %v", err)
			}

			if !op.Since.Equal(tt.want.Since) {
				t.Errorf("ApplyOpts() Since = %v, want %v", op.Since, tt.want.Since)
			}

			if !reflect.DeepEqual(op.SelectedComponents, tt.want.SelectedComponents) {
				t.Errorf("ApplyOpts() SelectedComponents = %v, want %v", op.SelectedComponents, tt.want.SelectedComponents)
			}
		})
	}
}
