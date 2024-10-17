package derpmap

import (
	"testing"
)

func TestDefaultDERPMap(t *testing.T) {
	if DefaultDERPMap.Regions == nil {
		t.Errorf("DefaultDERPMap.Regions is nil")
	}
	if len(DefaultDERPMap.Regions) == 0 {
		t.Errorf("DefaultDERPMap.Regions is empty")
	}
	for _, region := range DefaultDERPMap.Regions {
		if region.RegionID == 0 {
			t.Errorf("DefaultDERPMap.Regions[%d].RegionID is 0", region.RegionID)
		}
		if region.RegionCode == "" {
			t.Errorf("DefaultDERPMap.Regions[%d].RegionCode is empty", region.RegionID)
		}
		if region.RegionName == "" {
			t.Errorf("DefaultDERPMap.Regions[%d].RegionName is empty", region.RegionID)
		}
		if region.Nodes == nil {
			t.Errorf("DefaultDERPMap.Regions[%d].Nodes is nil", region.RegionID)
		}
		if len(region.Nodes) == 0 {
			t.Errorf("DefaultDERPMap.Regions[%d].Nodes is empty", region.RegionID)
		}
		t.Logf("region name %q has %d nodes", region.RegionName, len(region.Nodes))
	}
}
