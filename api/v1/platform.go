package v1

// GPUPlatformInfo preserves NVML's location fields without translating between
// physical slots, tray indices, and module identifiers. Numeric zero is valid.
type GPUPlatformInfo struct {
	ChassisSerial  string            `json:"chassisSerial,omitempty"`
	IBGUID         string            `json:"ibGuid,omitempty"`
	SlotNumber     *uint8            `json:"slotNumber,omitempty"`
	TrayIndex      *uint8            `json:"trayIndex,omitempty"`
	HostID         *uint8            `json:"hostId,omitempty"`
	PeerType       *uint8            `json:"peerType,omitempty"`
	ModuleID       *uint8            `json:"moduleId,omitempty"`
	DeviceModuleID *uint32           `json:"deviceModuleId,omitempty"`
	Version        uint32            `json:"version,omitempty"`
	Errors         map[string]string `json:"errors,omitempty"`
}
