package fuse

import (
	"reflect"
	"testing"
)

func Test_listConnections(t *testing.T) {
	expectedConnections := map[int]ConnectionInfo{
		44: {
			Device:               44,
			CongestionThreshold:  9,
			CongestedPercent:     0,
			MaxBackground:        12,
			MaxBackgroundPercent: 0,
			Waiting:              0,
		},
		53: {
			Device:               53,
			CongestionThreshold:  150,
			CongestedPercent:     0,
			MaxBackground:        200,
			MaxBackgroundPercent: 0,
			Waiting:              0,
		},
		82: {
			Device:               82,
			CongestionThreshold:  150,
			CongestedPercent:     0.6666666666666667,
			MaxBackground:        200,
			MaxBackgroundPercent: 0.5,
			Waiting:              1,
		},
		550: {
			Device:               550,
			CongestionThreshold:  150,
			CongestedPercent:     0,
			MaxBackground:        200,
			MaxBackgroundPercent: 0,
			Waiting:              0,
		},
	}

	infos, err := listConnections("./test/connections")
	if err != nil {
		t.Fatal(err)
	}

	for _, info := range infos {
		if expected, ok := expectedConnections[info.Device]; !ok {
			t.Errorf("unexpected connection: %+v", info)
		} else if !reflect.DeepEqual(info, expected) {
			t.Errorf("unexpected connection: %+v (expected: %+v)", info, expected)
		}
	}
}
