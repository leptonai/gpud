package class

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfiniBandClass(t *testing.T) {
	fs, err := newClassDirInterface("testdata/sys-class-infiniband-h100.0")
	assert.NoError(t, err)

	ibc, err := loadDevices(fs, nil)
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
	// Result depends on whether the system has InfiniBand hardware
	_, err = LoadDevices("")
	if _, statErr := os.Stat(DefaultRootDir); os.IsNotExist(statErr) {
		// Default directory doesn't exist - should return error
		assert.Error(t, err, "Expected error when default directory doesn't exist")
	} else {
		// Default directory exists - may or may not have devices
		// Just verify it doesn't panic or crash
		t.Logf("Default InfiniBand directory exists, LoadDevices returned err=%v", err)
	}

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
	_, err := loadDevices(mockFS, nil)
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
	_, err := parseInfiniBandDevice(mockFS, "test_device", nil)
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
	_, err = parseInfiniBandDevice(mockFS, "test_device", nil)
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
	_, err = parseInfiniBandDevice(mockFS, "test_device", nil)
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
	device, err := parseInfiniBandDevice(mockFS, "test_device", nil)
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", device.FirmwareVersion)
	assert.Equal(t, "", device.BoardID)
	assert.Equal(t, "", device.HCAType)
}

// TestParseInfiniBandPortErrors tests error cases in parseInfiniBandPort
func TestParseInfiniBandPortErrors(t *testing.T) {
	// Test invalid port number - this test is no longer relevant since parseInfiniBandPort now takes uint
	// Instead, test with a mock that simulates the error condition
	mockFS := &mockClassDirInterface{
		files: map[string]string{
			"test_device/ports/1/link_layer": "InfiniBand",
		},
		readErrors: map[string]error{
			"test_device/ports/1/link_layer": errors.New("link_layer read error"),
		},
	}
	_, err := parseInfiniBandPort(mockFS, "test_device", 1, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "link_layer read error")

	// Test error reading state
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/ports/1/link_layer": "InfiniBand",
		},
		readErrors: map[string]error{
			"test_device/ports/1/state": errors.New("state read error"),
		},
	}
	_, err = parseInfiniBandPort(mockFS, "test_device", 1, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "state read error")

	// Test error parsing state
	mockFS = &mockClassDirInterface{
		files: map[string]string{
			"test_device/ports/1/link_layer": "InfiniBand",
			"test_device/ports/1/state":      "invalid state format",
		},
	}
	_, err = parseInfiniBandPort(mockFS, "test_device", 1, nil)
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
	_, err = parseInfiniBandPort(mockFS, "test_device", 1, nil)
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
	_, err = parseInfiniBandPort(mockFS, "test_device", 1, nil)
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
	_, err = parseInfiniBandPort(mockFS, "test_device", 1, nil)
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

	counters, err := parseInfiniBandCounters(mockFS, "test_port", nil)
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

	counters, err = parseInfiniBandCounters(mockFS, "test_port", nil)
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

	counters, err = parseInfiniBandCounters(mockFS, "test_port", nil)
	assert.NoError(t, err)
	assert.Nil(t, counters.LinkDowned) // Should be nil due to permission error
	assert.NotNil(t, counters.PortXmitData)
	assert.Equal(t, uint64(8000), *counters.PortXmitData) // Should be multiplied by 4
}

func TestParseInfiniBandCountersCachesEINVALInIgnoreFiles(t *testing.T) {
	t.Parallel()

	portDir := filepath.Join("test_device", "ports", "1")
	countersDir := filepath.Join(portDir, "counters")
	counterName := "port_rcv_data"
	counterPath := filepath.Join(countersDir, counterName)

	ignoreFiles := make(map[string]struct{})
	op := &Op{}
	WithIgnoreFiles(ignoreFiles)(op)

	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			countersDir: {
				mockDirEntry{name: counterName, dir: false},
			},
		},
		readErrors: map[string]error{
			counterPath: syscall.EINVAL,
		},
	}

	_, err := parseInfiniBandCounters(mockFS, portDir, op)
	require.NoError(t, err)

	_, ok := ignoreFiles[counterPath]
	assert.True(t, ok)

	// Ensure ignored files are skipped on subsequent reads.
	mockFS = &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			countersDir: {
				mockDirEntry{name: counterName, dir: false},
			},
		},
		readErrors: map[string]error{
			counterPath: errors.New("should not read ignored file"),
		},
	}

	_, err = parseInfiniBandCounters(mockFS, portDir, op)
	require.NoError(t, err)
}

func TestParseInfiniBandHwCountersCachesEINVALInIgnoreFiles(t *testing.T) {
	t.Parallel()

	portDir := filepath.Join("test_device", "ports", "1")
	hwCountersDir := filepath.Join(portDir, "hw_counters")
	counterName := "lifespan"
	counterPath := filepath.Join(hwCountersDir, counterName)

	ignoreFiles := make(map[string]struct{})
	op := &Op{}
	WithIgnoreFiles(ignoreFiles)(op)

	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			hwCountersDir: {
				mockDirEntry{name: counterName, dir: false},
			},
		},
		readErrors: map[string]error{
			counterPath: syscall.EINVAL,
		},
	}

	_, err := parseInfiniBandHwCounters(mockFS, portDir, op)
	require.NoError(t, err)

	_, ok := ignoreFiles[counterPath]
	assert.True(t, ok)

	// Ensure ignored files are skipped on subsequent reads.
	mockFS = &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			hwCountersDir: {
				mockDirEntry{name: counterName, dir: false},
			},
		},
		readErrors: map[string]error{
			counterPath: errors.New("should not read ignored file"),
		},
	}

	_, err = parseInfiniBandHwCounters(mockFS, portDir, op)
	require.NoError(t, err)
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

	hwCounters, err := parseInfiniBandHwCounters(mockFS, "test_port", nil)
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

	_, err = parseInfiniBandHwCounters(mockFS, "test_port", nil)
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

	devices, err := loadDevices(mockFS, nil)
	assert.NoError(t, err)
	assert.Equal(t, 4, len(devices))

	// Check sorting order
	assert.Equal(t, "mlx5_1", devices[0].Name)
	assert.Equal(t, "mlx5_10", devices[1].Name)
	assert.Equal(t, "mlx5_2", devices[2].Name)
	assert.Equal(t, "mlx5_9", devices[3].Name)
}

// TestDeviceNameFiltering tests that only devices with "mlx" prefix are included
func TestDeviceNameFiltering(t *testing.T) {
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"": {
				mockDirEntry{name: "mlx5_0", dir: true},      // Should be included
				mockDirEntry{name: "mlx5_1", dir: true},      // Should be included
				mockDirEntry{name: "mlx4_0", dir: true},      // Should be included
				mockDirEntry{name: "._mlx5_0", dir: true},    // Should be excluded (starts with ._)
				mockDirEntry{name: "bond0", dir: true},       // Should be excluded (no mlx prefix)
				mockDirEntry{name: "eth0", dir: true},        // Should be excluded (no mlx prefix)
				mockDirEntry{name: "ib0", dir: true},         // Should be excluded (no mlx prefix)
				mockDirEntry{name: "some_mlx5_0", dir: true}, // Should be excluded (mlx not at start)
				mockDirEntry{name: "MLX5_0", dir: true},      // Should be excluded (case sensitive)
			},
		},
		files: map[string]string{
			"mlx5_0/fw_ver": "1.0",
			"mlx5_1/fw_ver": "1.0",
			"mlx4_0/fw_ver": "1.0",
		},
	}

	// Add empty ports directories for valid devices
	for _, device := range []string{"mlx5_0", "mlx5_1", "mlx4_0"} {
		mockFS.dirs[filepath.Join(device, "ports")] = []os.DirEntry{}
	}

	devices, err := loadDevices(mockFS, nil)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(devices), "Should only include devices with 'mlx' prefix")

	// Check that only the correct devices are included
	deviceNames := make([]string, len(devices))
	for i, device := range devices {
		deviceNames[i] = device.Name
	}
	assert.Contains(t, deviceNames, "mlx5_0")
	assert.Contains(t, deviceNames, "mlx5_1")
	assert.Contains(t, deviceNames, "mlx4_0")
	assert.NotContains(t, deviceNames, "._mlx5_0")
	assert.NotContains(t, deviceNames, "bond0")
	assert.NotContains(t, deviceNames, "eth0")
	assert.NotContains(t, deviceNames, "ib0")
	assert.NotContains(t, deviceNames, "some_mlx5_0")
	assert.NotContains(t, deviceNames, "MLX5_0")
}

// TestLoadDevicesWithMixedDeviceTypes tests loading devices with mixed valid/invalid types
func TestLoadDevicesWithMixedDeviceTypes(t *testing.T) {
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"": {
				mockDirEntry{name: "mlx5_0", dir: true},
				mockDirEntry{name: "bond0", dir: true},
				mockDirEntry{name: "mlx5_1", dir: true},
				mockDirEntry{name: "eth0", dir: true},
			},
		},
		files: map[string]string{
			"mlx5_0/fw_ver": "28.41.1000",
			"mlx5_1/fw_ver": "28.41.1000",
			// bond0 and eth0 don't have fw_ver files as they're not InfiniBand devices
		},
	}

	// Add empty ports directories for InfiniBand devices only
	for _, device := range []string{"mlx5_0", "mlx5_1"} {
		mockFS.dirs[filepath.Join(device, "ports")] = []os.DirEntry{}
	}

	devices, err := loadDevices(mockFS, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(devices), "Should only load InfiniBand devices")

	// Verify device properties
	for _, device := range devices {
		assert.True(t, strings.HasPrefix(device.Name, "mlx"))
		assert.Equal(t, "28.41.1000", device.FirmwareVersion)
	}
}

// TestLoadDevicesEmptyDirectory tests behavior when the infiniband directory is empty
func TestLoadDevicesEmptyDirectory(t *testing.T) {
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"": {}, // Empty directory
		},
	}

	devices, err := loadDevices(mockFS, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(devices), "Should return empty slice for empty directory")
}

// TestLoadDevicesOnlyNonInfiniBandDevices tests when only non-InfiniBand devices are present
func TestLoadDevicesOnlyNonInfiniBandDevices(t *testing.T) {
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"": {
				mockDirEntry{name: "bond0", dir: true},
				mockDirEntry{name: "eth0", dir: true},
				mockDirEntry{name: "ib0", dir: true},
				mockDirEntry{name: "wlan0", dir: true},
			},
		},
	}

	devices, err := loadDevices(mockFS, nil)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(devices), "Should return empty slice when no InfiniBand devices found")
}

// TestDeviceNameCaseSensitivity tests that device name filtering is case sensitive
func TestDeviceNameCaseSensitivity(t *testing.T) {
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"": {
				mockDirEntry{name: "mlx5_0", dir: true}, // Should be included
				mockDirEntry{name: "MLX5_0", dir: true}, // Should be excluded
				mockDirEntry{name: "Mlx5_0", dir: true}, // Should be excluded
				mockDirEntry{name: "mLx5_0", dir: true}, // Should be excluded
			},
		},
		files: map[string]string{
			"mlx5_0/fw_ver": "1.0",
		},
	}

	mockFS.dirs[filepath.Join("mlx5_0", "ports")] = []os.DirEntry{}

	devices, err := loadDevices(mockFS, nil)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(devices), "Should only include lowercase 'mlx' prefixed devices")
	assert.Equal(t, "mlx5_0", devices[0].Name)
}

// TestLoadDevicesWithFilesInsteadOfDirectories tests error handling when files exist instead of directories
func TestLoadDevicesWithFilesInsteadOfDirectories(t *testing.T) {
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"": {
				mockDirEntry{name: "mlx5_0", dir: true},  // Valid directory
				mockDirEntry{name: "mlx5_1", dir: false}, // File instead of directory - should cause error
			},
		},
		files: map[string]string{
			"mlx5_0/fw_ver": "1.0",
		},
	}

	mockFS.dirs[filepath.Join("mlx5_0", "ports")] = []os.DirEntry{}

	// The loadDevices function now returns an error when it encounters a file instead of a directory
	// because parseInfiniBandDevice will try to read fw_ver from what it thinks is a device directory
	devices, err := loadDevices(mockFS, nil)
	assert.Error(t, err, "Should return error when encountering file instead of directory")
	assert.Contains(t, err.Error(), "failed to read HCA firmware version")
	assert.Nil(t, devices)
}

// TestLoadDevicesWithExcludedDevices tests the WithExcludedDevices option
// This is useful for excluding devices with restricted PFs that cause ACCESS_REG errors
// ref. https://github.com/prometheus/node_exporter/issues/3434
// ref. https://github.com/leptonai/gpud/issues/1164
func TestLoadDevicesWithExcludedDevices(t *testing.T) {
	mockFS := &mockClassDirInterface{
		dirs: map[string][]os.DirEntry{
			"": {
				mockDirEntry{name: "mlx5_0", dir: true},
				mockDirEntry{name: "mlx5_1", dir: true},
				mockDirEntry{name: "mlx5_2", dir: true},
			},
		},
		files: map[string]string{
			"mlx5_0/fw_ver": "28.41.1000",
			"mlx5_1/fw_ver": "28.41.1000",
			"mlx5_2/fw_ver": "28.41.1000",
		},
	}

	// Add empty ports directories for each device
	for _, device := range []string{"mlx5_0", "mlx5_1", "mlx5_2"} {
		mockFS.dirs[filepath.Join(device, "ports")] = []os.DirEntry{}
	}

	// Test without exclusions - should return all 3 devices
	devices, err := loadDevices(mockFS, nil)
	require.NoError(t, err)
	assert.Equal(t, 3, len(devices), "Should return all 3 devices when no exclusions")

	// Test with exclusion of mlx5_0 - should return 2 devices
	op := &Op{
		excludedDevices: map[string]struct{}{
			"mlx5_0": {},
		},
	}
	devices, err = loadDevices(mockFS, op)
	require.NoError(t, err)
	assert.Equal(t, 2, len(devices), "Should return 2 devices when mlx5_0 is excluded")
	for _, d := range devices {
		assert.NotEqual(t, "mlx5_0", d.Name, "mlx5_0 should be excluded")
	}

	// Test with exclusion of multiple devices - should return 1 device
	op = &Op{
		excludedDevices: map[string]struct{}{
			"mlx5_0": {},
			"mlx5_1": {},
		},
	}
	devices, err = loadDevices(mockFS, op)
	require.NoError(t, err)
	assert.Equal(t, 1, len(devices), "Should return 1 device when mlx5_0 and mlx5_1 are excluded")
	assert.Equal(t, "mlx5_2", devices[0].Name)

	// Test with exclusion of all devices - should return empty slice
	op = &Op{
		excludedDevices: map[string]struct{}{
			"mlx5_0": {},
			"mlx5_1": {},
			"mlx5_2": {},
		},
	}
	devices, err = loadDevices(mockFS, op)
	require.NoError(t, err)
	assert.Equal(t, 0, len(devices), "Should return 0 devices when all are excluded")

	// Test with non-existent device in exclusion list - should be ignored
	op = &Op{
		excludedDevices: map[string]struct{}{
			"mlx5_99": {},
		},
	}
	devices, err = loadDevices(mockFS, op)
	require.NoError(t, err)
	assert.Equal(t, 3, len(devices), "Should return all 3 devices when excluded device doesn't exist")
}

// TestWithExcludedDevicesOption tests the WithExcludedDevices option function
func TestWithExcludedDevicesOption(t *testing.T) {
	// Test that the option correctly builds the exclusion map
	op := &Op{}
	opt := WithExcludedDevices([]string{"mlx5_0", "mlx5_1"})
	opt(op)

	assert.NotNil(t, op.excludedDevices)
	assert.Equal(t, 2, len(op.excludedDevices))
	_, ok := op.excludedDevices["mlx5_0"]
	assert.True(t, ok, "mlx5_0 should be in excluded devices")
	_, ok = op.excludedDevices["mlx5_1"]
	assert.True(t, ok, "mlx5_1 should be in excluded devices")

	// Test with empty list
	op = &Op{}
	opt = WithExcludedDevices([]string{})
	opt(op)
	assert.Equal(t, 0, len(op.excludedDevices))

	// Test with nil list
	op = &Op{}
	opt = WithExcludedDevices(nil)
	opt(op)
	assert.Equal(t, 0, len(op.excludedDevices))
}
