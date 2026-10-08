package machineinfo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"k8s.io/utils/cpuset"
)

type cpuTopology struct {
	cpusPerCore int64
	numCores    int64
	numSockets  int64
}

// readCPUTopology counts only cores and sockets represented by online CPUs.
// Sibling sets identify cores without assuming core IDs are globally unique.
func readCPUTopology(ctx context.Context, sysfs fs.FS) (cpuTopology, error) {
	online, err := readCPUSet(sysfs, "online")
	if err != nil {
		return cpuTopology{}, err
	}
	if online.IsEmpty() {
		return cpuTopology{}, fmt.Errorf("no online CPUs")
	}

	type core struct {
		siblings cpuset.CPUSet
		socket   int64
	}
	cores := make(map[int]*core, online.Size())
	sockets := make(map[int64]struct{})
	var topology cpuTopology
	for _, id := range online.UnsortedList() {
		if err := ctx.Err(); err != nil {
			return cpuTopology{}, err
		}
		base := fmt.Sprintf("cpu%d/topology/", id)
		data, err := fs.ReadFile(sysfs, base+"physical_package_id")
		if err != nil {
			return cpuTopology{}, err
		}
		socket, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil || socket < 0 {
			return cpuTopology{}, fmt.Errorf("invalid physical package ID for CPU %d: %q", id, data)
		}

		siblings, err := readCPUSet(sysfs, base+"core_cpus_list")
		if errors.Is(err, fs.ErrNotExist) {
			siblings, err = readCPUSet(sysfs, base+"thread_siblings_list")
		}
		if err != nil {
			return cpuTopology{}, err
		}
		if !siblings.IsSubsetOf(online) {
			siblings = siblings.Intersection(online)
		}
		if !siblings.Contains(id) {
			return cpuTopology{}, fmt.Errorf("CPU %d is missing from its online sibling set", id)
		}
		if known := cores[id]; known != nil {
			if known.socket != socket || !known.siblings.Equals(siblings) {
				return cpuTopology{}, fmt.Errorf("inconsistent sibling topology for CPU %d", id)
			}
			continue
		}

		group := &core{siblings: siblings, socket: socket}
		for _, sibling := range siblings.UnsortedList() {
			if cores[sibling] != nil {
				return cpuTopology{}, fmt.Errorf("overlapping sibling topology for CPU %d", id)
			}
			cores[sibling] = group
		}
		threads := int64(siblings.Size())
		if topology.numCores == 0 {
			topology.cpusPerCore = threads
		} else if topology.cpusPerCore != threads {
			// Zero remains unknown even if subsequent cores have matching counts.
			topology.cpusPerCore = 0
		}
		topology.numCores++
		sockets[socket] = struct{}{}
	}

	// CPU hotplug must not turn a partial snapshot into reported topology.
	currentOnline, err := readCPUSet(sysfs, "online")
	if err != nil {
		return cpuTopology{}, err
	}
	if !online.Equals(currentOnline) {
		return cpuTopology{}, fmt.Errorf("online CPUs changed while collecting topology")
	}
	topology.numSockets = int64(len(sockets))
	return topology, nil
}

func readCPUSet(sysfs fs.FS, name string) (cpuset.CPUSet, error) {
	data, err := fs.ReadFile(sysfs, name)
	if err != nil {
		return cpuset.CPUSet{}, err
	}
	set, err := cpuset.Parse(strings.TrimSpace(string(data)))
	if err != nil {
		return cpuset.CPUSet{}, fmt.Errorf("invalid CPU list in %s: %w", name, err)
	}
	return set, nil
}
