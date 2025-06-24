package class

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfiniBandClass(t *testing.T) {
	fs, err := newClassDirInterface("testdata/sys-class-infiniband-h100.0")
	assert.NoError(t, err)

	ibc, err := loadDevices(fs)
	assert.NoError(t, err)

	// Verify the total number of devices
	assert.Equal(t, 9, len(ibc))

	// Find mlx5_1 device
	var device *Device
	for i := range ibc {
		if ibc[i].Name == "mlx5_1" {
			device = &ibc[i]
			break
		}
	}
	assert.NotNil(t, device, "mlx5_1 device should exist")
	assert.Equal(t, "mlx5_1", device.Name)
	assert.Equal(t, "MT_0000000838", device.BoardID)
	assert.Equal(t, "28.41.1000", device.FirmwareVersion)
	assert.Equal(t, "MT4129", device.HCAType)

	// Find port 1 in the slice
	var port1 *Port
	for i := range device.Ports {
		if device.Ports[i].Port == 1 {
			port1 = &device.Ports[i]
			break
		}
	}
	assert.NotNil(t, port1, "Port 1 should exist")
	assert.Equal(t, "mlx5_1", port1.Name)

	assert.Equal(t, uint(1), port1.Port)

	assert.Equal(t, "ACTIVE", port1.State)
	assert.Equal(t, uint(4), port1.StateID)

	assert.Equal(t, "LinkUp", port1.PhysState)
	assert.Equal(t, uint(5), port1.PhysStateID)

	// Verify rate is parsed correctly from "400 Gb/sec (4X NDR)"
	// 400 Gb/s * 125000000 = 50000000000 bytes/second
	assert.Equal(t, 400.0, port1.RateGBSec)
	assert.Equal(t, uint64(50000000000), port1.Rate)

	assert.Equal(t, uint64(255), *port1.Counters.LinkDowned)
	assert.Equal(t, uint64(15), *port1.Counters.ExcessiveBufferOverrunErrors)
	assert.Equal(t, uint64(2), *port1.Counters.PortRcvSwitchRelayErrors)
}

// TestLoadDevices tests the public LoadDevices function
func TestLoadDevices(t *testing.T) {
	// Test with valid directory
	devices, err := LoadDevices("testdata/sys-class-infiniband-h100.0")
	require.NoError(t, err)
	assert.Equal(t, 9, len(devices))

	// Test with empty string (should use default)
	_, err = LoadDevices("")
	// This will fail if default directory doesn't exist, which is expected
	assert.Error(t, err)

	// Test with non-existent directory
	_, err = LoadDevices("/non/existent/path")
	assert.Error(t, err)
}

// TestParseState tests the parseState function with various inputs
func TestParseState(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedID  uint
		expectedStr string
		expectError bool
	}{
		{
			name:        "valid state",
			input:       "4: ACTIVE",
			expectedID:  4,
			expectedStr: "ACTIVE",
			expectError: false,
		},
		{
			name:        "valid state with spaces",
			input:       "  5  :  LinkUp  ",
			expectedID:  5,
			expectedStr: "LinkUp",
			expectError: false,
		},
		{
			name:        "invalid format - no colon",
			input:       "4 ACTIVE",
			expectError: true,
		},
		{
			name:        "invalid format - multiple colons",
			input:       "4:ACTIVE:extra",
			expectError: true,
		},
		{
			name:        "invalid ID",
			input:       "abc: ACTIVE",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, str, err := parseState(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedID, id)
				assert.Equal(t, tt.expectedStr, str)
			}
		})
	}
}

// TestParseRate tests the parseRate function with various inputs
func TestParseRate(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expectedRateGBSec float64
		expectedRate      uint64
		expectError       bool
	}{
		{
			name:              "valid rate - 100 Gb/sec",
			input:             "100 Gb/sec (4X EDR)",
			expectedRateGBSec: 100.0,
			expectedRate:      12500000000, // 100 * 125000000
			expectError:       false,
		},
		{
			name:              "valid rate - 400 Gb/sec",
			input:             "400 Gb/sec (4X NDR)",
			expectedRateGBSec: 400.0,
			expectedRate:      50000000000, // 400 * 125000000
			expectError:       false,
		},
		{
			name:              "valid rate with decimal",
			input:             "25.78125 Gb/sec",
			expectedRateGBSec: 25.78125,
			expectedRate:      3222656250, // 25.78125 * 125000000
			expectError:       false,
		},
		{
			name:        "invalid format - no space",
			input:       "100Gb/sec",
			expectError: true,
		},
		{
			name:        "invalid format - no unit",
			input:       "100",
			expectError: true,
		},
		{
			name:        "invalid number",
			input:       "abc Gb/sec",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rateGBSec, rate, err := parseRate(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedRateGBSec, rateGBSec)
				assert.Equal(t, tt.expectedRate, rate)
			}
		})
	}
}

// mockClassDirInterface is a mock implementation for testing error cases
type mockClassDirInterface struct {
	files       map[string]string
	dirs        map[string][]os.DirEntry
	readErrors  map[string]error
	listErrors  map[string]error
	existsMap   map[string]bool
	existsError map[string]error
}

type mockDirEntry struct {
	name string
	dir  bool
}

func (m mockDirEntry) Name() string { return m.name }
func (m mockDirEntry) IsDir() bool  { return m.dir }
func (m mockDirEntry) Type() os.FileMode {
	if m.dir {
		return os.ModeDir
	}
	return 0
}
func (m mockDirEntry) Info() (os.FileInfo, error) { return nil, nil }

func (m *mockClassDirInterface) readFile(path string) (string, error) {
	if err, ok := m.readErrors[path]; ok {
		return "", err
	}
	if content, ok := m.files[path]; ok {
		return content, nil
	}
	return "", os.ErrNotExist
}

func (m *mockClassDirInterface) listDir(path string) ([]os.DirEntry, error) {
	if err, ok := m.listErrors[path]; ok {
		return nil, err
	}
	if entries, ok := m.dirs[path]; ok {
		return entries, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockClassDirInterface) exists(path string) (bool, error) {
	if err, ok := m.existsError[path]; ok {
		return false, err
	}
	if exists, ok := m.existsMap[path]; ok {
		return exists, nil
	}
	return false, nil
}

// TestLoadDevicesErrors tests error cases in loadDevices
func TestLoadDevicesErrors(t *testing.T) {
	// Test listDir error
	mockFS := &mockClassDirInterface{
		listErrors: map[string]error{
			"": errors.New("list error"),
		},
	}
	_, err := loadDevices(mockFS)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

// TestParseInfiniBandDeviceErrors tests error cases in parseInfiniBandDevice
func TestParseInfiniBandDeviceErrors(t *testing.T) {
	// Test missing fw_ver
	mockFS := &mockClassDirInterface{
		readErrors: map[string]error{
			"test_device/fw_ver": errors.New("read error"),
		},
	}
	_, err := parseInfiniBandDevice(mockFS, "test_device")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read HCA firmware version")

	// Test error reading optional files (board_id, hca_type)
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/fw_ver": "1.0.0",
		},
		readErrors: map[string]error{
			"test_device/board_id": errors.New("permission denied"),
		},
	}
	_, err = parseInfiniBandDevice(mockFS, "test_device")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read file")

	// Test error listing ports
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/fw_ver":   "1.0.0",
			"test_device/board_id": "TEST_BOARD",
			"test_device/hca_type": "TEST_HCA",
		},
		listErrors: map[string]error{
			"test_device/ports": errors.New("ports list error"),
		},
	}
	_, err = parseInfiniBandDevice(mockFS, "test_device")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list InfiniBand ports")

	// Test with optional files missing (should succeed)
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/fw_ver": "1.0.0",
		},
		readErrors: map[string]error{
			"test_device/board_id": os.ErrNotExist,
			"test_device/hca_type": os.ErrNotExist,
		},
		dirs: map[string][]os.DirEntry{
			"test_device/ports": {},
		},
	}
	device, err := parseInfiniBandDevice(mockFS, "test_device")
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", device.FirmwareVersion)
	assert.Equal(t, "", device.BoardID)
	assert.Equal(t, "", device.HCAType)
}

// TestParseInfiniBandPortErrors tests error cases in parseInfiniBandPort
func TestParseInfiniBandPortErrors(t *testing.T) {
	// Test invalid port number
	_, err := parseInfiniBandPort(nil, "test_device", "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to convert")

	// Test error reading state
	mockFS := &mockClassDirInterface{
		files: map[string]string{
			"test_device/ports/1/link_layer": "InfiniBand",
		},
		readErrors: map[string]error{
			"test_device/ports/1/state": errors.New("state read error"),
		},
	}
	_, err = parseInfiniBandPort(mockFS, "test_device", "1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "state read error")

	// Test error parsing state
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/ports/1/link_layer": "InfiniBand",
			"test_device/ports/1/state":      "invalid state format",
		},
	}
	_, err = parseInfiniBandPort(mockFS, "test_device", "1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not parse state file")

	// Test error reading phys_state
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/ports/1/link_layer": "InfiniBand",
			"test_device/ports/1/state":      "4: ACTIVE",
		},
		readErrors: map[string]error{
			"test_device/ports/1/phys_state": errors.New("phys_state read error"),
		},
	}
	_, err = parseInfiniBandPort(mockFS, "test_device", "1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "phys_state read error")

	// Test error parsing rate
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/ports/1/link_layer": "InfiniBand",
			"test_device/ports/1/state":      "4: ACTIVE",
			"test_device/ports/1/phys_state": "5: LinkUp",
			"test_device/ports/1/rate":       "invalid rate",
		},
	}
	_, err = parseInfiniBandPort(mockFS, "test_device", "1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not parse rate file")

	// Test with counters directory exists error
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/ports/1/link_layer": "InfiniBand",
			"test_device/ports/1/state":      "4: ACTIVE",
			"test_device/ports/1/phys_state": "5: LinkUp",
			"test_device/ports/1/rate":       "100 Gb/sec",
		},
		existsError: map[string]error{
			"test_device/ports/1/counters": errors.New("exists check error"),
		},
	}
	_, err = parseInfiniBandPort(mockFS, "test_device", "1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exists check error")
}

// TestParseInfiniBandCountersEdgeCases tests edge cases in counter parsing
func TestParseInfiniBandCountersEdgeCases(t *testing.T) {
	// Test with N/A counter values (should be skipped)
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"test_port/counters": {
				mockDirEntry{name: "link_downed", dir: false},
				mockDirEntry{name: "port_rcv_data", dir: false},
			},
		},
		files: map[string]string{
			"test_port/counters/link_downed":   "N/A (no PMA)",
			"test_port/counters/port_rcv_data": "1000",
		},
	}

	counters, err := parseInfiniBandCounters(mockFS, "test_port")
	assert.NoError(t, err)
	assert.Nil(t, counters.LinkDowned) // Should be nil due to N/A
	assert.NotNil(t, counters.PortRcvData)
	assert.Equal(t, uint64(4000), *counters.PortRcvData) // Should be multiplied by 4

	// Test with directory entries (should be skipped)
	mockFS = &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"test_port/counters": {
				mockDirEntry{name: "subdir", dir: true},
				mockDirEntry{name: "link_downed", dir: false},
			},
		},
		files: map[string]string{
			"test_port/counters/link_downed": "100",
		},
	}

	counters, err = parseInfiniBandCounters(mockFS, "test_port")
	assert.NoError(t, err)
	assert.NotNil(t, counters.LinkDowned)
	assert.Equal(t, uint64(100), *counters.LinkDowned)

	// Test with permission errors (should be skipped)
	mockFS = &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"test_port/counters": {
				mockDirEntry{name: "link_downed", dir: false},
				mockDirEntry{name: "port_xmit_data", dir: false},
			},
		},
		files: map[string]string{
			"test_port/counters/port_xmit_data": "2000",
		},
		readErrors: map[string]error{
			"test_port/counters/link_downed": os.ErrPermission,
		},
	}

	counters, err = parseInfiniBandCounters(mockFS, "test_port")
	assert.NoError(t, err)
	assert.Nil(t, counters.LinkDowned) // Should be nil due to permission error
	assert.NotNil(t, counters.PortXmitData)
	assert.Equal(t, uint64(8000), *counters.PortXmitData) // Should be multiplied by 4
}

// TestParseInfiniBandHwCountersEdgeCases tests edge cases in HW counter parsing
func TestParseInfiniBandHwCountersEdgeCases(t *testing.T) {
	// Test with N/A counter values (should be skipped)
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"test_port/hw_counters": {
				mockDirEntry{name: "lifespan", dir: false},
				mockDirEntry{name: "out_of_buffer", dir: false},
			},
		},
		files: map[string]string{
			"test_port/hw_counters/lifespan":      "N/A (no PMA)",
			"test_port/hw_counters/out_of_buffer": "50",
		},
	}

	hwCounters, err := parseInfiniBandHwCounters(mockFS, "test_port")
	assert.NoError(t, err)
	assert.Nil(t, hwCounters.Lifespan) // Should be nil due to N/A
	assert.NotNil(t, hwCounters.OutOfBuffer)
	assert.Equal(t, uint64(50), *hwCounters.OutOfBuffer)

	// Test error listing directory
	mockFS = &mockClassDirInterface{
		listErrors: map[string]error{
			"test_port/hw_counters": errors.New("list error"),
		},
	}

	_, err = parseInfiniBandHwCounters(mockFS, "test_port")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list error")
}

// TestDevicesSorting tests that devices are sorted by name
func TestDevicesSorting(t *testing.T) {
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"": {
				mockDirEntry{name: "mlx5_9", dir: true},
				mockDirEntry{name: "mlx5_1", dir: true},
				mockDirEntry{name: "mlx5_10", dir: true},
				mockDirEntry{name: "mlx5_2", dir: true},
			},
		},
		files: map[string]string{
			"mlx5_9/fw_ver":  "1.0",
			"mlx5_1/fw_ver":  "1.0",
			"mlx5_10/fw_ver": "1.0",
			"mlx5_2/fw_ver":  "1.0",
		},
	}

	// Add empty ports directories for each device
	for _, device := range []string{"mlx5_9", "mlx5_1", "mlx5_10", "mlx5_2"} {
		mockFS.dirs[filepath.Join(device, "ports")] = []os.DirEntry{}
	}

	devices, err := loadDevices(mockFS)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(devices))

	// Check sorting order
	assert.Equal(t, "mlx5_1", devices[0].Name)
	assert.Equal(t, "mlx5_10", devices[1].Name)
	assert.Equal(t, "mlx5_2", devices[2].Name)
	assert.Equal(t, "mlx5_9", devices[3].Name)
}
