package db

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNewOrderedMap(t *testing.T) {
	tests := []struct {
		name    string
		input   map[string]string
		wantLen int
	}{
		{
			name:    "empty map",
			input:   map[string]string{},
			wantLen: 0,
		},
		{
			name:    "single entry",
			input:   map[string]string{"key": "value"},
			wantLen: 1,
		},
		{
			name: "multiple entries",
			input: map[string]string{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newOrderedMap(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("newOrderedMap() got len = %v, want %v", len(got), tt.wantLen)
			}

			// Verify all key-value pairs are present
			for k, v := range tt.input {
				found := false
				for _, entry := range got {
					if entry.K == k && entry.V == v {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("newOrderedMap() missing entry {%v: %v}", k, v)
				}
			}
		})
	}
}

func TestDecodeOrderedMap(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    map[string]string
		wantErr bool
	}{
		{
			name:    "empty array",
			input:   []byte("[]"),
			want:    map[string]string{},
			wantErr: false,
		},
		{
			name:    "single entry",
			input:   []byte(`[{"k":"key1","v":"value1"}]`),
			want:    map[string]string{"key1": "value1"},
			wantErr: false,
		},
		{
			name:    "multiple entries",
			input:   []byte(`[{"k":"key1","v":"value1"},{"k":"key2","v":"value2"}]`),
			want:    map[string]string{"key1": "value1", "key2": "value2"},
			wantErr: false,
		},
		{
			name:    "invalid json",
			input:   []byte(`invalid json`),
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := decodeOrderedMap(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeOrderedMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("decodeOrderedMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderedMap_ToMap(t *testing.T) {
	tests := []struct {
		name string
		m    orderedMap
		want map[string]string
	}{
		{
			name: "empty map",
			m:    orderedMap{},
			want: map[string]string{},
		},
		{
			name: "single entry",
			m:    orderedMap{{K: "key1", V: "value1"}},
			want: map[string]string{"key1": "value1"},
		},
		{
			name: "multiple entries",
			m: orderedMap{
				{K: "key1", V: "value1"},
				{K: "key2", V: "value2"},
			},
			want: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.toMap(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("orderedMap.toMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderedMap_MarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		m       orderedMap
		want    string
		wantErr bool
	}{
		{
			name:    "empty map",
			m:       orderedMap{},
			want:    "[]",
			wantErr: false,
		},
		{
			name:    "single entry",
			m:       orderedMap{{K: "key1", V: "value1"}},
			want:    `[{"k":"key1","v":"value1"}]`,
			wantErr: false,
		},
		{
			name: "multiple entries (tests ordering)",
			m: orderedMap{
				{K: "key2", V: "value2"},
				{K: "key1", V: "value1"},
				{K: "key3", V: "value3"},
			},
			want:    `[{"k":"key1","v":"value1"},{"k":"key2","v":"value2"},{"k":"key3","v":"value3"}]`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.m.marshalJSON()
			if (err != nil) != tt.wantErr {
				t.Errorf("orderedMap.marshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Compare JSON after normalizing both strings
			var gotJSON, wantJSON interface{}
			if err := json.Unmarshal(got, &gotJSON); err != nil {
				t.Errorf("Failed to unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantJSON); err != nil {
				t.Errorf("Failed to unmarshal expected: %v", err)
			}

			if !reflect.DeepEqual(gotJSON, wantJSON) {
				t.Errorf("orderedMap.marshalJSON() = %v, want %v", string(got), tt.want)
			}
		})
	}
}
