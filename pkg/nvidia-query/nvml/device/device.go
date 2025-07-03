// Package device provides a wrapper around the "github.com/NVIDIA/go-nvlib/pkg/nvlib/device".Device
// type that adds a PCIBusID method.
package device

import (
	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
)

// Device is a wrapper around the "github.com/NVIDIA/go-nvlib/pkg/nvlib/device".Device
// type that adds a PCIBusID method.
type Device interface {
	device.Device
	PCIBusID() string
}

var _ Device = &nvDevice{}

type nvDevice struct {
	device.Device
	busID string
}

func (d *nvDevice) PCIBusID() string {
	return d.busID
}

func New(dev device.Device, busID string) Device {
	return &nvDevice{Device: dev, busID: busID}
}
