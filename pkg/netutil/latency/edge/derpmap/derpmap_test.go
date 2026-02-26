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

func TestGetRegionCode_Aliases(t *testing.T) {
	tests := map[string]string{
		"Bangalore": "ap-south-1",
		"Bengaluru": "ap-south-1",
	}

	for name, want := range tests {
		got, ok := GetRegionCode(name)
		if !ok {
			t.Fatalf("expected region mapping for %q", name)
		}
		if got != want {
			t.Fatalf("region mapping for %q = %q, want %q", name, got, want)
		}
	}
}
