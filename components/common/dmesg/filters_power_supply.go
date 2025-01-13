package dmesg

import (
	power_supply_dmesg "github.com/leptonai/gpud/components/power-supply/dmesg"
	power_supply_id "github.com/leptonai/gpud/components/power-supply/id"
	query_log_common "github.com/leptonai/gpud/components/query/log/common"

	"k8s.io/utils/ptr"
)

const (
	EventPowerSupplyInsufficientPowerOnPCIe = "power_supply_insufficient_power_on_pcie"
)

func DefaultDmesgFiltersForPowerSupply() []*query_log_common.Filter {
	return []*query_log_common.Filter{
		{
			Name:            EventPowerSupplyInsufficientPowerOnPCIe,
			Regex:           ptr.To(string(power_supply_dmesg.RegexInsufficientPowerOnPCIe)),
			OwnerReferences: []string{power_supply_id.Name},
		},
	}
}
