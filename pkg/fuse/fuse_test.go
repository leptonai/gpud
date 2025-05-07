package fuse

import (
	"encoding/json"
	"fmt"
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

func TestListConnectionsWithFinder(t *testing.T) {
	// Create a mock finder function
	mockFinder := func(minor int) (string, string, error) {
		deviceMap := map[int]struct {
			fsType string
			device string
		}{
			44:  {"fuse.test1", "test-device-1"},
			53:  {"fuse.test2", "test-device-2"},
			82:  {"fuse.test3", "test-device-3"},
			550: {"fuse.test4", "test-device-4"},
		}

		if info, ok := deviceMap[minor]; ok {
			return info.fsType, info.device, nil
		}
		return "", "", fmt.Errorf("device not found: %d", minor)
	}

	// Call the function with our mock finder and test directory
	infos, err := listConnectionsWithFinder(mockFinder, "./test/connections")
	if err != nil {
		t.Fatal(err)
	}

	// Check that we got the expected number of results
	if len(infos) != 4 {
		t.Fatalf("expected 4 connections, got %d", len(infos))
	}

	// Verify that fstype and device name are populated correctly
	expectedDeviceInfo := map[int]struct {
		fsType string
		device string
	}{
		44:  {"fuse.test1", "test-device-1"},
		53:  {"fuse.test2", "test-device-2"},
		82:  {"fuse.test3", "test-device-3"},
		550: {"fuse.test4", "test-device-4"},
	}

	for _, info := range infos {
		expected, ok := expectedDeviceInfo[info.Device]
		if !ok {
			t.Errorf("unexpected device: %d", info.Device)
			continue
		}

		if info.Fstype != expected.fsType {
			t.Errorf("device %d: fstype = %q, want %q", info.Device, info.Fstype, expected.fsType)
		}

		if info.DeviceName != expected.device {
			t.Errorf("device %d: device name = %q, want %q", info.Device, info.DeviceName, expected.device)
		}
	}
}

func TestListConnections_ErrorHandling(t *testing.T) {
	// Create a mock finder function that returns an error
	mockFinder := func(minor int) (string, string, error) {
		return "", "", fmt.Errorf("mocked error")
	}

	// Call the function with our mock finder and test directory
	infos, err := listConnectionsWithFinder(mockFinder, "./test/connections")
	if err != nil {
		t.Fatal(err)
	}

	// Check that connections are still returned but with empty fstype and device name
	for _, info := range infos {
		if info.Fstype != "" {
			t.Errorf("expected empty fstype for device %d, got %q", info.Device, info.Fstype)
		}
		if info.DeviceName != "" {
			t.Errorf("expected empty device name for device %d, got %q", info.Device, info.DeviceName)
		}
	}
}

func TestListConnectionsWithFinder_InvalidDirectory(t *testing.T) {
	// Use a directory that doesn't exist
	mockFinder := func(minor int) (string, string, error) {
		return "mock-fs", "mock-device", nil
	}

	_, err := listConnectionsWithFinder(mockFinder, "./nonexistent-directory")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
}

func TestListConnections(t *testing.T) {
	// Save the original implementation
	originalListConnections := defaultListConnections

	defer func() {
		// Restore the original implementation
		defaultListConnections = originalListConnections
	}()

	// Replace with mock implementation
	defaultListConnections = func() (ConnectionInfos, error) {
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
		}
		return infos, nil
	}

	// Call the function that should use our mock now
	infos, err := ListConnections()
	if err != nil {
		t.Fatal(err)
	}

	// Verify it returned our mock data
	if len(infos) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(infos))
	}

	if infos[0].Device != 42 {
		t.Errorf("expected device 42, got %d", infos[0].Device)
	}

	if infos[0].Fstype != "fuse.test1" {
		t.Errorf("expected fstype fuse.test1, got %s", infos[0].Fstype)
	}
}
