package machineinfo

import (
	"context"
	"fmt"
	"io/fs"
	"strconv"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/cpuset"
)

type topologyTestCore struct {
	socket int
	cpus   string
}

func topologyTestFS(t *testing.T, online string, groups []topologyTestCore) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{"online": {Data: []byte(online + "\n")}}
	for _, group := range groups {
		cpus, err := cpuset.Parse(group.cpus)
		require.NoError(t, err)
		for _, id := range cpus.UnsortedList() {
			base := fmt.Sprintf("cpu%d/topology/", id)
			files[base+"physical_package_id"] = &fstest.MapFile{Data: []byte(strconv.Itoa(group.socket) + "\n")}
			files[base+"core_cpus_list"] = &fstest.MapFile{Data: []byte(group.cpus + "\n")}
			// Reused IDs must not merge distinct sibling groups or sockets.
			files[base+"core_id"] = &fstest.MapFile{Data: []byte("0\n")}
		}
	}
	return files
}

func TestReadCPUTopology(t *testing.T) {
	tests := []struct {
		name   string
		online string
		groups []topologyTestCore
		want   cpuTopology
	}{
		{
			name:   "SMT enabled across sockets with reused core IDs",
			online: "0-7",
			groups: []topologyTestCore{{0, "0,4"}, {0, "1,5"}, {1, "2,6"}, {1, "3,7"}},
			want:   cpuTopology{cpusPerCore: 2, numCores: 4, numSockets: 2},
		},
		{
			name:   "SMT disabled",
			online: "0-3",
			groups: []topologyTestCore{{0, "0"}, {0, "1"}, {1, "2"}, {1, "3"}},
			want:   cpuTopology{cpusPerCore: 1, numCores: 4, numSockets: 2},
		},
		{
			name:   "offline siblings cores and socket excluded",
			online: "2,8",
			groups: []topologyTestCore{{3, "2-3"}, {3, "8-9"}, {3, "10-11"}, {7, "12-13"}},
			want:   cpuTopology{cpusPerCore: 1, numCores: 2, numSockets: 1},
		},
		{
			name:   "asymmetric online siblings are not averaged",
			online: "0-3",
			groups: []topologyTestCore{{0, "0-1"}, {0, "2"}, {0, "3"}},
			want:   cpuTopology{numCores: 3, numSockets: 1},
		},
		{
			name:   "partial offline SMT leaves multiplier unknown",
			online: "0-2",
			groups: []topologyTestCore{{0, "0,2"}, {1, "1,3"}},
			want:   cpuTopology{numCores: 2, numSockets: 2},
		},
		{
			name:   "more than two hardware threads per core",
			online: "0-7",
			groups: []topologyTestCore{{0, "0-3"}, {0, "4-7"}},
			want:   cpuTopology{cpusPerCore: 4, numCores: 2, numSockets: 1},
		},
		{
			name:   "unknown package keeps measurable cores",
			online: "0-3",
			groups: []topologyTestCore{{-1, "0,2"}, {-1, "1,3"}},
			want:   cpuTopology{cpusPerCore: 2, numCores: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := topologyTestFS(t, tt.online, tt.groups)
			got, err := readCPUTopology(context.Background(), files)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReadCPUTopologyLegacySiblingLists(t *testing.T) {
	files := topologyTestFS(t, "0-3", []topologyTestCore{{0, "0,2"}, {0, "1,3"}})
	for id := range 4 {
		base := fmt.Sprintf("cpu%d/topology/", id)
		files[base+"thread_siblings_list"] = files[base+"core_cpus_list"]
		delete(files, base+"core_cpus_list")
	}
	got, err := readCPUTopology(context.Background(), files)
	require.NoError(t, err)
	assert.Equal(t, cpuTopology{cpusPerCore: 2, numCores: 2, numSockets: 1}, got)
}

func TestReadCPUTopologyRejectsIncompleteOrInvalidData(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		value string
	}{
		{name: "missing online list", path: "online"},
		{name: "empty online list", path: "online", value: "\n"},
		{name: "malformed online list", path: "online", value: "0-x"},
		{name: "negative CPU", path: "online", value: "-1,0"},
		{name: "missing online CPU directory", path: "online", value: "0-4"},
		{name: "missing socket", path: "cpu3/topology/physical_package_id"},
		{name: "invalid socket", path: "cpu3/topology/physical_package_id", value: "socket0"},
		{name: "overflow socket", path: "cpu3/topology/physical_package_id", value: "9223372036854775808"},
		{name: "socket disagreement", path: "cpu3/topology/physical_package_id", value: "1"},
		{name: "missing siblings", path: "cpu3/topology/core_cpus_list"},
		{name: "empty siblings", path: "cpu3/topology/core_cpus_list", value: "\n"},
		{name: "invalid siblings", path: "cpu3/topology/core_cpus_list", value: "1-"},
		{name: "reversed sibling range", path: "cpu3/topology/core_cpus_list", value: "3-1"},
		{name: "siblings omit self", path: "cpu3/topology/core_cpus_list", value: "1"},
		{name: "asymmetric sibling membership", path: "cpu3/topology/core_cpus_list", value: "3"},
		{name: "overlapping sibling groups", path: "cpu3/topology/core_cpus_list", value: "0-3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := topologyTestFS(t, "0-3", []topologyTestCore{{0, "0,2"}, {0, "1,3"}})
			if tt.value == "" {
				delete(files, tt.path)
			} else {
				files[tt.path] = &fstest.MapFile{Data: []byte(tt.value)}
			}
			got, err := readCPUTopology(context.Background(), files)
			require.Error(t, err)
			assert.Equal(t, cpuTopology{}, got, "partial topology must not escape on errors")
		})
	}
}

func TestReadCPUTopologyDoesNotHideInvalidModernSiblingList(t *testing.T) {
	files := topologyTestFS(t, "0", []topologyTestCore{{0, "0"}})
	files["cpu0/topology/thread_siblings_list"] = files["cpu0/topology/core_cpus_list"]
	files["cpu0/topology/core_cpus_list"] = &fstest.MapFile{Data: []byte("invalid")}
	_, err := readCPUTopology(context.Background(), files)
	require.Error(t, err)
}

type changingOnlineFS struct {
	fs.FS
	reads int
}

func (f *changingOnlineFS) ReadFile(name string) ([]byte, error) {
	if name == "online" {
		f.reads++
		if f.reads > 1 {
			return []byte("0-2\n"), nil
		}
	}
	return fs.ReadFile(f.FS, name)
}

func TestReadCPUTopologyRejectsOnlineCPUChanges(t *testing.T) {
	files := topologyTestFS(t, "0-3", []topologyTestCore{{0, "0,2"}, {0, "1,3"}})
	got, err := readCPUTopology(context.Background(), &changingOnlineFS{FS: files})
	require.ErrorContains(t, err, "online CPUs changed")
	assert.Equal(t, cpuTopology{}, got)
}

func TestReadCPUTopologyCanceled(t *testing.T) {
	files := topologyTestFS(t, "0", []topologyTestCore{{0, "0"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := readCPUTopology(ctx, files)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, cpuTopology{}, got)
}
