package firmware

import (
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// instance is one firmware reading before grouping.
type instance struct {
	component string
	version   string
	// name is empty for host-level components, which report a count only.
	name string
}

// group collapses readings into one entry per (component, version).
// The control plane patches the Machine whenever the reported value changes,
// so entries and names are sorted to keep the output stable between reads.
func group(found []instance, readAt time.Time) []apiv1.MachineFirmware {
	if len(found) == 0 {
		return nil
	}

	type key struct{ component, version string }
	lastUpdated := metav1.NewTime(readAt.UTC().Truncate(time.Second))
	byKey := make(map[key]*apiv1.MachineFirmware)
	for _, f := range found {
		k := key{component: f.component, version: f.version}
		row, ok := byKey[k]
		if !ok {
			row = &apiv1.MachineFirmware{Component: f.component, Version: f.version, LastUpdated: lastUpdated}
			byKey[k] = row
		}
		row.Instances++
		if f.name != "" {
			row.InstanceNames = append(row.InstanceNames, f.name)
		}
	}

	rows := make([]apiv1.MachineFirmware, 0, len(byKey))
	for _, row := range byKey {
		names := row.InstanceNames
		sort.Slice(names, func(i, j int) bool { return naturalLess(names[i], names[j]) })
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Component != rows[j].Component {
			return rows[i].Component < rows[j].Component
		}
		return rows[i].Version < rows[j].Version
	})
	return rows
}

// naturalLess orders strings with embedded numbers by numeric value, so
// gpu2 < gpu10 and mlx5_9 < mlx5_10.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			numA, restA := splitDigits(a)
			numB, restB := splitDigits(b)
			if c := compareNumeric(numA, numB); c != 0 {
				return c < 0
			}
			a, b = restA, restB
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

// compareNumeric compares two digit runs by value, then by length so "1"
// sorts before "01".
func compareNumeric(a, b string) int {
	ta, tb := trimLeadingZeros(a), trimLeadingZeros(b)
	switch {
	case len(ta) != len(tb):
		return len(ta) - len(tb)
	case ta != tb:
		if ta < tb {
			return -1
		}
		return 1
	default:
		return len(a) - len(b)
	}
}

func trimLeadingZeros(s string) string {
	for len(s) > 1 && s[0] == '0' {
		s = s[1:]
	}
	return s
}

func splitDigits(s string) (string, string) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
