package nvlink

import (
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// RemoteEndpoint is NVML's logical endpoint, not a physical rack port mapping.
type RemoteEndpoint struct {
	PCIBusID       string            `json:"pci_bus_id,omitempty"`
	DeviceType     string            `json:"device_type,omitempty"`
	DeviceTypeCode *uint32           `json:"device_type_code,omitempty"`
	Errors         map[string]string `json:"errors,omitempty"`
}

func getRemoteEndpoint(dev device.Device, link int) *RemoteEndpoint {
	r := &RemoteEndpoint{Errors: make(map[string]string)}
	pci, ret := dev.GetNvLinkRemotePciInfo(link)
	if ret == nvml.SUCCESS {
		for _, b := range pci.BusId {
			if b == 0 {
				break
			}
			r.PCIBusID += string(b)
		}
	} else {
		r.Errors["pci_bus_id"] = nvml.ErrorString(ret)
	}
	typ, ret := dev.GetNvLinkRemoteDeviceType(link)
	if ret == nvml.SUCCESS {
		code := uint32(typ)
		r.DeviceTypeCode = &code
		switch typ {
		case nvml.NVLINK_DEVICE_TYPE_GPU:
			r.DeviceType = "gpu"
		case nvml.NVLINK_DEVICE_TYPE_SWITCH:
			r.DeviceType = "switch"
		case nvml.NVLINK_DEVICE_TYPE_IBMNPU:
			r.DeviceType = "ibm_npu"
		default:
			r.DeviceType = "unknown"
		}
	} else {
		r.Errors["device_type"] = nvml.ErrorString(ret)
	}
	return r
}

// FieldCounter preserves NVML's identity and sample time. NVLink 5 counters
// deliberately do not reuse the names or semantics of removed legacy counters.
type FieldCounter struct {
	FieldID   uint32  `json:"field_id"`
	ScopeID   uint32  `json:"scope_id"`
	Timestamp int64   `json:"timestamp_us"`
	Value     *uint64 `json:"value,omitempty"`
	Error     string  `json:"error,omitempty"`
}

func getLinkFieldCounters(dev device.Device, link int) map[string]FieldCounter {
	fields := []struct {
		name string
		id   uint32
	}{
		{"recovery_successful_events", nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS},
		{"recovery_failed_events", nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS},
		{"recovery_events", nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_EVENTS},
		{"effective_errors", nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS},
		{"symbol_errors", nvml.FI_DEV_NVLINK_COUNT_SYMBOL_ERRORS},
		{"xmit_bytes", nvml.FI_DEV_NVLINK_COUNT_XMIT_BYTES},
		{"rcv_bytes", nvml.FI_DEV_NVLINK_COUNT_RCV_BYTES},
	}
	values := make([]nvml.FieldValue, len(fields))
	for i, f := range fields {
		values[i] = nvml.FieldValue{FieldId: f.id, ScopeId: uint32(link)}
	}
	ret := dev.GetFieldValues(values)
	result := make(map[string]FieldCounter, len(fields))
	for i, f := range fields {
		v := values[i]
		c := FieldCounter{FieldID: f.id, ScopeID: uint32(link), Timestamp: v.Timestamp}
		switch {
		case ret != nvml.SUCCESS:
			c.Error = nvml.ErrorString(ret)
		case nvml.Return(v.NvmlReturn) != nvml.SUCCESS:
			c.Error = nvml.ErrorString(nvml.Return(v.NvmlReturn))
		default:
			var value uint64
			switch nvml.ValueType(v.ValueType) {
			case nvml.VALUE_TYPE_UNSIGNED_LONG_LONG, nvml.VALUE_TYPE_UNSIGNED_LONG:
				value = binary.NativeEndian.Uint64(v.Value[:])
			case nvml.VALUE_TYPE_UNSIGNED_INT:
				value = uint64(binary.NativeEndian.Uint32(v.Value[:]))
			default:
				c.Error = fmt.Sprintf("unexpected NVML value type %d", v.ValueType)
			}
			if c.Error == "" {
				c.Value = &value
			}
		}
		result[f.name] = c
	}
	return result
}

// MarshalJSON omits unavailable numeric observations instead of fabricating zeros.
func (s NVLinkState) MarshalJSON() ([]byte, error) {
	type plain NVLinkState
	b, err := json.Marshal(plain(s))
	if err != nil {
		return nil, err
	}
	var data map[string]json.RawMessage
	if err = json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	for key := range s.Errors {
		delete(data, key)
	}
	if _, failed := s.Errors["state"]; failed {
		delete(data, "feature_enabled")
	}
	return json.Marshal(data)
}

func (s NVLinkStates) counterAvailable(key string) bool {
	if len(s) == 0 {
		return false
	}
	for _, link := range s {
		if _, failed := link.Errors[key]; failed {
			return false
		}
		if _, failed := link.Errors["state"]; failed {
			return false
		}
	}
	return true
}
