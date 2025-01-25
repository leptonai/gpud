package db

import (
	"encoding/json"
	"sort"
)

// order keys in encoded json map to make select statement work
type mapEntry struct {
	K string `json:"k"`
	V string `json:"v"`
}

type orderedMap []mapEntry

func newOrderedMap(m map[string]string) orderedMap {
	entries := make([]mapEntry, 0, len(m))
	for k, v := range m {
		entries = append(entries, mapEntry{K: k, V: v})
	}
	return entries
}

func decodeOrderedMap(b []byte) (map[string]string, error) {
	var m orderedMap
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m.toMap(), nil
}

func (m orderedMap) toMap() map[string]string {
	result := make(map[string]string)
	for _, entry := range m {
		result[entry.K] = entry.V
	}
	return result
}

func (m orderedMap) marshalJSON() ([]byte, error) {
	sort.Slice(m, func(i, j int) bool {
		return m[i].K < m[j].K
	})
	return json.Marshal(m)
}
