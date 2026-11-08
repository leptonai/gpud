package v1

import (
	"encoding/json"
	"slices"
	"strings"
)

// FaultScope is the scope of the observed failure, not a claim about its cause.
type FaultScope string

const (
	FaultScopeGPU          FaultScope = "gpu"
	FaultScopeNVLinkDomain FaultScope = "nvlink_domain"
	FaultScopeIMEXDomain   FaultScope = "imex_domain"
)

func (s *HealthState) SetAffectedGPUs(uuids ...string) {
	ids := slices.Clone(uuids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	ids = slices.DeleteFunc(ids, func(id string) bool { return id == "" })
	if len(ids) == 0 {
		return
	}
	if s.ExtraInfo == nil {
		s.ExtraInfo = make(map[string]string)
	}
	encoded, _ := json.Marshal(ids)
	s.ExtraInfo["gpu_uuids"] = string(encoded)
	var missing []string
	for _, id := range ids {
		if !strings.Contains(s.Reason, id) {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		s.Reason += " (GPUs: " + strings.Join(missing, ", ") + ")"
	}
}
