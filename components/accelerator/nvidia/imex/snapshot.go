// Package imex reports IMEX domain connectivity without changing the domain.
package imex

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Peer struct {
	NodeID   int    `json:"node_id"`
	Address  string `json:"address"`
	Hostname string `json:"hostname,omitempty"`
	Daemon   string `json:"daemon"`
	Status   string `json:"status"`
}

type Snapshot struct {
	Daemon      string `json:"daemon"`
	NodeID      int    `json:"node_id"`
	DomainState string `json:"domain_state"`
	Peers       []Peer `json:"peers"`
	// Matrix is directed: a healthy observer can identify an unreachable peer.
	Matrix map[int]map[int]string `json:"matrix"`
}

var nodeLine = regexp.MustCompile(`^Node\s+#(\d+)\s+([*-])\s+(\S+)\s*\*?\s+-\s+(\S+)\s+-\s+Version:(.*)$`)
var statuses = map[string]string{"C": "connected", "N": "never_connected", "R": "recovering", "D": "disconnected", "A": "authenticating", "I": "unavailable", "!V!": "version_mismatch", "!M!": "node_map_mismatch", "!A!": "authentication_error", "!R!": "waiting_for_recovery"}

// ParseSnapshot accepts the documented single-snapshot (-N) text format. It
// rejects incomplete or unfamiliar output instead of manufacturing success.
// https://docs.nvidia.com/multi-node-nvlink-systems/imex-guide/cmdservice.html
func ParseSnapshot(output string) (*Snapshot, error) {
	s := &Snapshot{NodeID: -1, Matrix: make(map[int]map[int]string)}
	nodes := make(map[int]Peer)
	var columns []int
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if m := nodeLine.FindStringSubmatch(line); m != nil {
			id, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, err
			}
			if _, exists := nodes[id]; exists {
				return nil, fmt.Errorf("duplicate node %d", id)
			}
			p := Peer{NodeID: id, Address: m[3], Daemon: strings.Trim(m[4], "*")}
			if _, host, ok := strings.Cut(m[5], "Hostname:"); ok {
				p.Hostname = strings.TrimSpace(host)
			}
			nodes[id] = p
			if m[2] == "*" {
				if s.NodeID >= 0 {
					return nil, fmt.Errorf("multiple local nodes")
				}
				s.NodeID = id
			}
			continue
		}
		if strings.HasPrefix(line, `Nodes From\To`) {
			if columns != nil {
				return nil, fmt.Errorf("multiple connectivity matrices")
			}
			for _, token := range strings.Fields(strings.TrimPrefix(line, `Nodes From\To`)) {
				id, err := strconv.Atoi(token)
				if err != nil {
					return nil, fmt.Errorf("invalid matrix column %q", token)
				}
				for _, previous := range columns {
					if previous == id {
						return nil, fmt.Errorf("duplicate matrix column %d", id)
					}
				}
				columns = append(columns, id)
			}
			continue
		}
		if strings.HasPrefix(line, "Domain State:") {
			s.DomainState = strings.TrimSpace(strings.TrimPrefix(line, "Domain State:"))
			continue
		}
		if len(columns) > 0 && line != "" && line[0] >= '0' && line[0] <= '9' {
			fields := strings.Fields(line)
			if len(fields) != len(columns)+1 {
				return nil, fmt.Errorf("incomplete connectivity row")
			}
			id, err := strconv.Atoi(fields[0])
			if err != nil {
				return nil, err
			}
			if _, exists := s.Matrix[id]; exists {
				return nil, fmt.Errorf("duplicate matrix row %d", id)
			}
			row := make(map[int]string)
			for i, column := range columns {
				status, ok := statuses[strings.Trim(fields[i+1], "*")]
				if !ok {
					return nil, fmt.Errorf("unknown connectivity status %q", fields[i+1])
				}
				row[column] = status
			}
			s.Matrix[id] = row
		}
	}
	if s.NodeID < 0 || len(nodes) == 0 || len(columns) != len(nodes) || len(s.Matrix) != len(nodes) || s.DomainState == "" {
		return nil, fmt.Errorf("incomplete IMEX snapshot")
	}
	local, ok := nodes[s.NodeID]
	if !ok {
		return nil, fmt.Errorf("local node not listed")
	}
	s.Daemon = local.Daemon
	for _, id := range columns {
		p, ok := nodes[id]
		if !ok || s.Matrix[id] == nil {
			return nil, fmt.Errorf("missing node or matrix row %d", id)
		}
		if id != s.NodeID {
			p.Status = s.Matrix[s.NodeID][id]
			s.Peers = append(s.Peers, p)
		}
	}
	sort.Slice(s.Peers, func(i, j int) bool { return s.Peers[i].NodeID < s.Peers[j].NodeID })
	return s, nil
}
