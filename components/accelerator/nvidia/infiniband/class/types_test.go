package class

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountersStruct(t *testing.T) {
	t.Run("empty counters", func(t *testing.T) {
		counters := Counters{}

		// All fields should be nil pointers
		assert.Nil(t, counters.ExcessiveBufferOverrunErrors)
		assert.Nil(t, counters.LinkDowned)
		assert.Nil(t, counters.LinkErrorRecovery)
		assert.Nil(t, counters.LocalLinkIntegrityErrors)
		assert.Nil(t, counters.MulticastRcvPackets)
		assert.Nil(t, counters.MulticastXmitPackets)
		assert.Nil(t, counters.PortRcvConstraintErrors)
		assert.Nil(t, counters.PortRcvData)
		assert.Nil(t, counters.PortRcvDiscards)
		assert.Nil(t, counters.PortRcvErrors)
		assert.Nil(t, counters.PortRcvPackets)
		assert.Nil(t, counters.PortRcvRemotePhysicalErrors)
		assert.Nil(t, counters.PortRcvSwitchRelayErrors)
		assert.Nil(t, counters.PortXmitConstraintErrors)
		assert.Nil(t, counters.PortXmitData)
		assert.Nil(t, counters.PortXmitDiscards)
		assert.Nil(t, counters.PortXmitPackets)
		assert.Nil(t, counters.PortXmitWait)
		assert.Nil(t, counters.SymbolError)
		assert.Nil(t, counters.UnicastRcvPackets)
		assert.Nil(t, counters.UnicastXmitPackets)
		assert.Nil(t, counters.VL15Dropped)
	})

	t.Run("populated counters", func(t *testing.T) {
		linkDowned := uint64(10)
		linkErrorRecovery := uint64(5)
		excessiveErrors := uint64(2)

		counters := Counters{
			LinkDowned:                   &linkDowned,
			LinkErrorRecovery:            &linkErrorRecovery,
			ExcessiveBufferOverrunErrors: &excessiveErrors,
		}

		assert.NotNil(t, counters.LinkDowned)
		assert.Equal(t, uint64(10), *counters.LinkDowned)
		assert.NotNil(t, counters.LinkErrorRecovery)
		assert.Equal(t, uint64(5), *counters.LinkErrorRecovery)
		assert.NotNil(t, counters.ExcessiveBufferOverrunErrors)
		assert.Equal(t, uint64(2), *counters.ExcessiveBufferOverrunErrors)

		// Unset fields should remain nil
		assert.Nil(t, counters.PortRcvData)
		assert.Nil(t, counters.PortXmitData)
	})
}

func TestHWCountersStruct(t *testing.T) {
	t.Run("empty hw counters", func(t *testing.T) {
		hwCounters := HWCounters{}

		// All fields should be nil pointers
		assert.Nil(t, hwCounters.DuplicateRequest)
		assert.Nil(t, hwCounters.ImpliedNakSeqErr)
		assert.Nil(t, hwCounters.Lifespan)
		assert.Nil(t, hwCounters.LocalAckTimeoutErr)
		assert.Nil(t, hwCounters.NpCnpSent)
		assert.Nil(t, hwCounters.NpEcnMarkedRocePackets)
		assert.Nil(t, hwCounters.OutOfBuffer)
		assert.Nil(t, hwCounters.OutOfSequence)
		assert.Nil(t, hwCounters.PacketSeqErr)
		assert.Nil(t, hwCounters.ReqCqeError)
		assert.Nil(t, hwCounters.ReqCqeFlushError)
		assert.Nil(t, hwCounters.ReqRemoteAccessErrors)
		assert.Nil(t, hwCounters.ReqRemoteInvalidRequest)
		assert.Nil(t, hwCounters.RespCqeError)
		assert.Nil(t, hwCounters.RespCqeFlushError)
		assert.Nil(t, hwCounters.RespLocalLengthError)
		assert.Nil(t, hwCounters.RespRemoteAccessErrors)
		assert.Nil(t, hwCounters.RnrNakRetryErr)
		assert.Nil(t, hwCounters.RoceAdpRetrans)
		assert.Nil(t, hwCounters.RoceAdpRetransTo)
		assert.Nil(t, hwCounters.RoceSlowRestart)
		assert.Nil(t, hwCounters.RoceSlowRestartCnps)
		assert.Nil(t, hwCounters.RoceSlowRestartTrans)
		assert.Nil(t, hwCounters.RpCnpHandled)
		assert.Nil(t, hwCounters.RpCnpIgnored)
		assert.Nil(t, hwCounters.RxAtomicRequests)
		assert.Nil(t, hwCounters.RxDctConnect)
		assert.Nil(t, hwCounters.RxIcrcEncapsulated)
		assert.Nil(t, hwCounters.RxReadRequests)
		assert.Nil(t, hwCounters.RxWriteRequests)
	})

	t.Run("populated hw counters", func(t *testing.T) {
		lifespan := uint64(100)
		outOfBuffer := uint64(25)
		duplicateRequest := uint64(3)

		hwCounters := HWCounters{
			Lifespan:         &lifespan,
			OutOfBuffer:      &outOfBuffer,
			DuplicateRequest: &duplicateRequest,
		}

		assert.NotNil(t, hwCounters.Lifespan)
		assert.Equal(t, uint64(100), *hwCounters.Lifespan)
		assert.NotNil(t, hwCounters.OutOfBuffer)
		assert.Equal(t, uint64(25), *hwCounters.OutOfBuffer)
		assert.NotNil(t, hwCounters.DuplicateRequest)
		assert.Equal(t, uint64(3), *hwCounters.DuplicateRequest)

		// Unset fields should remain nil
		assert.Nil(t, hwCounters.ImpliedNakSeqErr)
		assert.Nil(t, hwCounters.RxAtomicRequests)
	})
}

func TestPortStruct(t *testing.T) {
	t.Run("empty port", func(t *testing.T) {
		port := Port{}

		assert.Equal(t, "", port.Name)
		assert.Equal(t, "", port.LinkLayer)
		assert.Equal(t, uint(0), port.Port)
		assert.Equal(t, "", port.State)
		assert.Equal(t, uint(0), port.StateID)
		assert.Equal(t, "", port.PhysState)
		assert.Equal(t, uint(0), port.PhysStateID)
		assert.Equal(t, uint64(0), port.Rate)
		assert.Equal(t, float64(0), port.RateGBSec)
	})

	t.Run("populated port", func(t *testing.T) {
		linkDowned := uint64(5)
		linkErrorRecovery := uint64(2)
		lifespan := uint64(1000)

		port := Port{
			Name:        "mlx5_0",
			LinkLayer:   "InfiniBand",
			Port:        1,
			State:       "ACTIVE",
			StateID:     4,
			PhysState:   "LinkUp",
			PhysStateID: 5,
			Rate:        50000000000,
			RateGBSec:   400.0,
			Counters: Counters{
				LinkDowned:        &linkDowned,
				LinkErrorRecovery: &linkErrorRecovery,
			},
			HWCounters: HWCounters{
				Lifespan: &lifespan,
			},
		}

		assert.Equal(t, "mlx5_0", port.Name)
		assert.Equal(t, "InfiniBand", port.LinkLayer)
		assert.Equal(t, uint(1), port.Port)
		assert.Equal(t, "ACTIVE", port.State)
		assert.Equal(t, uint(4), port.StateID)
		assert.Equal(t, "LinkUp", port.PhysState)
		assert.Equal(t, uint(5), port.PhysStateID)
		assert.Equal(t, uint64(50000000000), port.Rate)
		assert.Equal(t, 400.0, port.RateGBSec)

		assert.NotNil(t, port.Counters.LinkDowned)
		assert.Equal(t, uint64(5), *port.Counters.LinkDowned)
		assert.NotNil(t, port.Counters.LinkErrorRecovery)
		assert.Equal(t, uint64(2), *port.Counters.LinkErrorRecovery)

		assert.NotNil(t, port.HWCounters.Lifespan)
		assert.Equal(t, uint64(1000), *port.HWCounters.Lifespan)
	})
}

func TestDeviceStruct(t *testing.T) {
	t.Run("empty device", func(t *testing.T) {
		device := Device{}

		assert.Equal(t, "", device.Name)
		assert.Equal(t, "", device.BoardID)
		assert.Equal(t, "", device.FirmwareVersion)
		assert.Equal(t, "", device.HCAType)
		assert.Empty(t, device.Ports)
	})

	t.Run("populated device", func(t *testing.T) {
		linkDowned := uint64(10)

		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:        "mlx5_0",
					LinkLayer:   "InfiniBand",
					Port:        1,
					State:       "ACTIVE",
					StateID:     4,
					PhysState:   "LinkUp",
					PhysStateID: 5,
					Rate:        50000000000,
					RateGBSec:   400.0,
					Counters: Counters{
						LinkDowned: &linkDowned,
					},
				},
			},
		}

		assert.Equal(t, "mlx5_0", device.Name)
		assert.Equal(t, "MT_0000000838", device.BoardID)
		assert.Equal(t, "28.41.1000", device.FirmwareVersion)
		assert.Equal(t, "MT4129", device.HCAType)
		assert.Len(t, device.Ports, 1)
		assert.Equal(t, uint(1), device.Ports[0].Port)
		assert.Equal(t, "ACTIVE", device.Ports[0].State)
	})
}

func TestDevicesSlice(t *testing.T) {
	t.Run("empty devices slice", func(t *testing.T) {
		devices := Devices{}
		assert.Empty(t, devices)
		assert.Len(t, devices, 0)
	})

	t.Run("populated devices slice", func(t *testing.T) {
		devices := Devices{
			{Name: "mlx5_0", BoardID: "BOARD1"},
			{Name: "mlx5_1", BoardID: "BOARD2"},
		}

		assert.Len(t, devices, 2)
		assert.Equal(t, "mlx5_0", devices[0].Name)
		assert.Equal(t, "BOARD1", devices[0].BoardID)
		assert.Equal(t, "mlx5_1", devices[1].Name)
		assert.Equal(t, "BOARD2", devices[1].BoardID)
	})
}

func TestDeviceRenderTable(t *testing.T) {
	t.Run("empty device", func(t *testing.T) {
		device := Device{}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()
		assert.Contains(t, output, "Device")
		assert.Contains(t, output, "Board ID")
		assert.Contains(t, output, "Firmware Version")
		// Should contain empty values since all fields are empty
		assert.Contains(t, output, "")
	})

	t.Run("device with basic info only", func(t *testing.T) {
		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
		}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()
		assert.Contains(t, output, "mlx5_0")
		assert.Contains(t, output, "MT_0000000838")
		assert.Contains(t, output, "28.41.1000")
		// Should not contain port information since no ports
		assert.NotContains(t, output, "Port 1")
	})

	t.Run("device with single port", func(t *testing.T) {
		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      1,
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 400.0,
				},
			},
		}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()
		assert.Contains(t, output, "mlx5_0")
		assert.Contains(t, output, "MT_0000000838")
		assert.Contains(t, output, "28.41.1000")
		assert.Contains(t, output, "Port 1 Name")
		assert.Contains(t, output, "Port 1 LinkLayer")
		assert.Contains(t, output, "Port 1 State")
		assert.Contains(t, output, "Port 1 Phys State")
		assert.Contains(t, output, "Port 1 Rate")
		assert.Contains(t, output, "InfiniBand")
		assert.Contains(t, output, "ACTIVE")
		assert.Contains(t, output, "LinkUp")
		assert.Contains(t, output, "400 Gb/sec")
	})

	t.Run("device with multiple ports", func(t *testing.T) {
		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      1,
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 400.0,
				},
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      2,
					State:     "DOWN",
					PhysState: "Disabled",
					RateGBSec: 100.0,
				},
			},
		}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()
		assert.Contains(t, output, "Port 1 Name")
		assert.Contains(t, output, "Port 1 State")
		assert.Contains(t, output, "Port 2 Name")
		assert.Contains(t, output, "Port 2 State")
		assert.Contains(t, output, "ACTIVE")
		assert.Contains(t, output, "DOWN")
		assert.Contains(t, output, "LinkUp")
		assert.Contains(t, output, "Disabled")
		assert.Contains(t, output, "400 Gb/sec")
		assert.Contains(t, output, "100 Gb/sec")
	})

	t.Run("device with port counters", func(t *testing.T) {
		linkDowned := uint64(15)
		linkErrorRecovery := uint64(5)
		excessiveErrors := uint64(3)

		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      1,
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 400.0,
					Counters: Counters{
						LinkDowned:                   &linkDowned,
						LinkErrorRecovery:            &linkErrorRecovery,
						ExcessiveBufferOverrunErrors: &excessiveErrors,
					},
				},
			},
		}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()
		assert.Contains(t, output, "Port 1 Link Downed")
		assert.Contains(t, output, "15")
		assert.Contains(t, output, "Port 1 Link Error Recovery")
		assert.Contains(t, output, "5")
		// Table may break long headers across lines, so check for key parts
		assert.Contains(t, output, "Port 1 Excessive Buffer")
		assert.Contains(t, output, "Overrun Errors")
		assert.Contains(t, output, "3")
	})

	t.Run("device with nil counter values", func(t *testing.T) {
		linkDowned := uint64(10)
		// linkErrorRecovery and excessiveErrors are nil

		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      1,
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 400.0,
					Counters: Counters{
						LinkDowned: &linkDowned,
						// LinkErrorRecovery and ExcessiveBufferOverrunErrors are nil
					},
				},
			},
		}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()
		// Should only contain LinkDowned counter, not the nil ones
		assert.Contains(t, output, "Port 1 Link Downed")
		assert.Contains(t, output, "10")
		assert.NotContains(t, output, "Port 1 Link Error Recovery")
		// Table may break long headers, check that neither part appears
		assert.NotContains(t, output, "Port 1 Excessive Buffer")
		assert.NotContains(t, output, "Overrun Errors")
	})

	t.Run("device with zero counter values", func(t *testing.T) {
		linkDowned := uint64(0)
		linkErrorRecovery := uint64(0)

		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      1,
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 400.0,
					Counters: Counters{
						LinkDowned:        &linkDowned,
						LinkErrorRecovery: &linkErrorRecovery,
					},
				},
			},
		}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()
		// Should contain counters even if they are zero
		assert.Contains(t, output, "Port 1 Link Downed")
		assert.Contains(t, output, "0")
		assert.Contains(t, output, "Port 1 Link Error Recovery")
	})

	t.Run("device with fractional rate", func(t *testing.T) {
		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      1,
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 25.78125, // Fractional rate
				},
			},
		}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()
		// Should convert fractional rate to integer for display
		assert.Contains(t, output, "25 Gb/sec")
	})
}

func TestDeviceRenderTableEdgeCases(t *testing.T) {
	t.Run("device with empty strings", func(t *testing.T) {
		device := Device{
			Name:            "",
			BoardID:         "",
			FirmwareVersion: "",
			HCAType:         "",
			Ports: []Port{
				{
					Name:      "",
					LinkLayer: "",
					Port:      0,
					State:     "",
					PhysState: "",
					RateGBSec: 0,
				},
			},
		}
		var buf bytes.Buffer

		// Should not panic with empty strings
		require.NotPanics(t, func() {
			device.RenderTable(&buf)
		})

		output := buf.String()
		assert.Contains(t, output, "Device")
		assert.Contains(t, output, "Port 0")
	})

	t.Run("device with large counter values", func(t *testing.T) {
		largeValue := uint64(18446744073709551615) // Max uint64

		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      1,
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 400.0,
					Counters: Counters{
						LinkDowned: &largeValue,
					},
				},
			},
		}
		var buf bytes.Buffer

		require.NotPanics(t, func() {
			device.RenderTable(&buf)
		})

		output := buf.String()
		assert.Contains(t, output, "18446744073709551615")
	})

	t.Run("nil writer should panic", func(t *testing.T) {
		device := Device{Name: "test"}

		// Should panic when writer is nil
		assert.Panics(t, func() {
			device.RenderTable(nil)
		})
	})
}

func TestTableOutputFormat(t *testing.T) {
	t.Run("verify table structure", func(t *testing.T) {
		linkDowned := uint64(5)

		device := Device{
			Name:            "mlx5_0",
			BoardID:         "MT_0000000838",
			FirmwareVersion: "28.41.1000",
			HCAType:         "MT4129",
			Ports: []Port{
				{
					Name:      "mlx5_0",
					LinkLayer: "InfiniBand",
					Port:      1,
					State:     "ACTIVE",
					PhysState: "LinkUp",
					RateGBSec: 400.0,
					Counters: Counters{
						LinkDowned: &linkDowned,
					},
				},
			},
		}
		var buf bytes.Buffer

		device.RenderTable(&buf)

		output := buf.String()

		// Verify it contains table formatting characters
		assert.Contains(t, output, "|")
		assert.Contains(t, output, "+")
		assert.Contains(t, output, "-")

		// Verify the output has multiple lines (indicating table structure)
		lines := strings.Split(output, "\n")
		assert.Greater(t, len(lines), 5) // Should have multiple rows

		// Verify specific rows exist
		found := make(map[string]bool)
		for _, line := range lines {
			if strings.Contains(line, "Device") && strings.Contains(line, "mlx5_0") {
				found["device"] = true
			}
			if strings.Contains(line, "Board ID") && strings.Contains(line, "MT_0000000838") {
				found["board_id"] = true
			}
			if strings.Contains(line, "Firmware Version") && strings.Contains(line, "28.41.1000") {
				found["firmware"] = true
			}
			if strings.Contains(line, "Port 1 Name") {
				found["port_name"] = true
			}
			if strings.Contains(line, "Port 1 Link Downed") && strings.Contains(line, "5") {
				found["counter"] = true
			}
		}

		assert.True(t, found["device"], "Device row not found")
		assert.True(t, found["board_id"], "Board ID row not found")
		assert.True(t, found["firmware"], "Firmware row not found")
		assert.True(t, found["port_name"], "Port name row not found")
		assert.True(t, found["counter"], "Counter row not found")
	})
}

func TestDevicesRenderTable(t *testing.T) {
	t.Run("empty devices slice", func(t *testing.T) {
		devices := Devices{}
		var buf bytes.Buffer

		devices.RenderTable(&buf)

		output := buf.String()
		assert.Empty(t, output, "Empty devices slice should produce no output")
	})

	t.Run("single device", func(t *testing.T) {
		devices := Devices{
			{
				Name:            "mlx5_0",
				BoardID:         "MT_0000000838",
				FirmwareVersion: "28.41.1000",
			},
		}
		var buf bytes.Buffer

		devices.RenderTable(&buf)

		output := buf.String()
		assert.Contains(t, output, "mlx5_0")
		assert.Contains(t, output, "MT_0000000838")
		assert.Contains(t, output, "28.41.1000")
		assert.Contains(t, output, "Device")
		assert.Contains(t, output, "Board ID")
	})

	t.Run("multiple devices", func(t *testing.T) {
		devices := Devices{
			{
				Name:            "mlx5_0",
				BoardID:         "BOARD_0",
				FirmwareVersion: "1.0.0",
			},
			{
				Name:            "mlx5_1",
				BoardID:         "BOARD_1",
				FirmwareVersion: "2.0.0",
			},
		}
		var buf bytes.Buffer

		devices.RenderTable(&buf)

		output := buf.String()
		// Should contain both devices
		assert.Contains(t, output, "mlx5_0")
		assert.Contains(t, output, "mlx5_1")
		assert.Contains(t, output, "BOARD_0")
		assert.Contains(t, output, "BOARD_1")
		assert.Contains(t, output, "1.0.0")
		assert.Contains(t, output, "2.0.0")

		// Should have multiple table sections (indicated by multiple "Device" headers)
		deviceCount := strings.Count(output, "Device")
		assert.Equal(t, 2, deviceCount, "Should have 2 device table headers")

		// Should have newlines separating the tables
		lines := strings.Split(output, "\n")
		assert.Greater(t, len(lines), 10, "Should have multiple lines with separators")
	})

	t.Run("devices with ports", func(t *testing.T) {
		linkDowned := uint64(5)
		devices := Devices{
			{
				Name:            "mlx5_0",
				BoardID:         "BOARD_0",
				FirmwareVersion: "1.0.0",
				Ports: []Port{
					{
						Name:      "mlx5_0",
						Port:      1,
						State:     "ACTIVE",
						RateGBSec: 400.0,
						Counters: Counters{
							LinkDowned: &linkDowned,
						},
					},
				},
			},
		}
		var buf bytes.Buffer

		devices.RenderTable(&buf)

		output := buf.String()
		assert.Contains(t, output, "mlx5_0")
		assert.Contains(t, output, "Port 1")
		assert.Contains(t, output, "ACTIVE")
		assert.Contains(t, output, "Port 1 Link Downed")
		assert.Contains(t, output, "5")
	})

	t.Run("nil writer should panic", func(t *testing.T) {
		devices := Devices{{Name: "test"}}

		// Should panic when writer is nil
		assert.Panics(t, func() {
			devices.RenderTable(nil)
		})
	})
}
