package disk

import (
	"os"
	"reflect"
	"testing"
)

func TestParseFindMntOutput(t *testing.T) {
	for _, file := range []string{"findmnt.0.json", "findmnt.1.json"} {
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatalf("error reading test data: %v", err)
		}
		output, err := ParseFindMntOutput(string(b))
		if err != nil {
			t.Fatalf("error finding mount target output: %v", err)
		}
		t.Logf("output: %+v", output)
	}
}

func TestExtractMntSources(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single source without brackets",
			input:    "/dev/sda1",
			expected: []string{"/dev/sda1"},
		},
		{
			name:     "source with path in brackets",
			input:    "/dev/mapper/vgroot-lvroot[/var/lib/lxc/ny2g2r14hh2-lxc/rootfs]",
			expected: []string{"/dev/mapper/vgroot-lvroot", "/var/lib/lxc/ny2g2r14hh2-lxc/rootfs"},
		},
		{
			name:     "source with simple path in brackets",
			input:    "/dev/mapper/lepton_vg-lepton_lv[/kubelet]",
			expected: []string{"/dev/mapper/lepton_vg-lepton_lv", "/kubelet"},
		},
		{
			name:     "multiple comma-separated sources",
			input:    "source1,source2[/path1,/path2]",
			expected: []string{"source1", "source2", "/path1", "/path2"},
		},
		{
			name:     "edge case with empty sections",
			input:    "[/path]",
			expected: []string{"/path"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := extractMntSources(tc.input)
			if !reflect.DeepEqual(result, tc.expected) {
				t.Errorf("extractMntSources(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}
