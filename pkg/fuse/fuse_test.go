package fuse

import (
	"encoding/json"
	"reflect"
	"strings"
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

func TestConnectionInfo_JSON(t *testing.T) {
	info := ConnectionInfo{
		Device:               42,
		Fstype:               "fuse.test",
		DeviceName:           "test-device",
		CongestionThreshold:  100,
		CongestedPercent:     25.5,
		MaxBackground:        200,
		MaxBackgroundPercent: 12.75,
		Waiting:              25,
	}

	expected := `{"device":42,"fstype":"fuse.test","device_name":"test-device","congestion_threshold":100,"congested_percent":25.5,"max_background":200,"max_background_percent":12.75,"waiting":25}`

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(data) != expected {
		t.Errorf("unexpected JSON output:\ngot:  %s\nwant: %s", string(data), expected)
	}
}

func TestConnectionInfos_RenderTable(t *testing.T) {
	infos := ConnectionInfos{
		{
			Device:               42,
			Fstype:               "fuse.test1",
			DeviceName:           "test-device-1",
			CongestionThreshold:  100,
			CongestedPercent:     25.5,
			MaxBackground:        200,
			MaxBackgroundPercent: 12.75,
			Waiting:              25,
		},
		{
			Device:               43,
			Fstype:               "fuse.test2",
			DeviceName:           "test-device-2",
			CongestionThreshold:  150,
			CongestedPercent:     0,
			MaxBackground:        300,
			MaxBackgroundPercent: 0,
			Waiting:              0,
		},
	}

	// Create a buffer to capture the table output
	var buf strings.Builder
	infos.RenderTable(&buf)

	// Check that the output contains expected headers and values
	output := buf.String()
	expectedStrings := []string{
		"Device", "Fstype", "Device Name", "Congestion Threshold", "Congested %",
		"Max Background Threshold", "Max Background %", "Waiting",
		"42", "fuse.test1", "test-device-1", "100", "25.50%", "200", "12.75%", "25",
		"43", "fuse.test2", "test-device-2", "150", "0.00%", "300", "0.00%", "0",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(strings.ToLower(output), strings.ToLower(expected)) {
			t.Errorf("expected table output to contain %q, but it didn't\nOutput:\n%s", expected, output)
		}
	}
}
