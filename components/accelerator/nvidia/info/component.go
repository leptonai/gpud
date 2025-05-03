// Package info provides relatively static information about the NVIDIA accelerator (e.g., GPU product names).
package info

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/log"
	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

// Name is the name of the component.
const Name = "accelerator-nvidia-info"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance       nvidianvml.Instance
	getDeviceCountFunc func() (int, error)
	getMemoryFunc      func(uuid string, dev device.Device) (nvidianvml.Memory, error)
	getSerialFunc      func(uuid string, dev device.Device) (string, error)
	getMinorIDFunc     func(uuid string, dev device.Device) (int, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:                cctx,
		cancel:             ccancel,
		nvmlInstance:       gpudInstance.NVMLInstance,
		getDeviceCountFunc: nvidiaquery.CountAllDevicesFromDevDir,
		getMemoryFunc:      nvidianvml.GetMemory,
		getSerialFunc:      nvidianvml.GetSerial,
		getMinorIDFunc:     nvidianvml.GetMinorID,
	}
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Start() error {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	return nil, nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu info")

	cr := &checkResult{
		ts: time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}

	cr.Product.Name = c.nvmlInstance.ProductName()
	cr.Product.Architecture = c.nvmlInstance.Architecture()
	cr.Product.Brand = c.nvmlInstance.Brand()

	cr.Driver.Version = c.nvmlInstance.DriverVersion()
	if cr.Driver.Version == "" {
		cr.err = fmt.Errorf("driver version is empty")
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "driver version is empty"
		return cr
	}

	cr.CUDA.Version = c.nvmlInstance.CUDAVersion()
	if cr.CUDA.Version == "" {
		cr.err = fmt.Errorf("CUDA version is empty")
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "CUDA version is empty"
		return cr
	}

	deviceCount, err := c.getDeviceCountFunc()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting device count"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr
	}
	cr.GPUCount.DeviceCount = deviceCount

	devs := c.nvmlInstance.Devices()
	cr.GPUCount.Attached = len(devs)

	for uuid, dev := range devs {
		if cr.Memory.TotalBytes == 0 {
			mem, err := c.getMemoryFunc(uuid, dev)
			if err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error getting memory"
				log.Logger.Errorw(cr.reason, "error", cr.err)
				return cr
			}
			cr.Memory.TotalBytes = mem.TotalBytes
			cr.Memory.TotalHumanized = mem.TotalHumanized
		}

		gpuID := GPUID{
			UUID: uuid,
		}

		if c.getSerialFunc != nil {
			serialID, err := c.getSerialFunc(uuid, dev)
			if err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error getting serial id"
				log.Logger.Errorw(cr.reason, "error", cr.err)
				return cr
			}
			gpuID.SN = serialID
		}

		if c.getMinorIDFunc != nil {
			minorID, err := c.getMinorIDFunc(uuid, dev)
			if err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error getting minor id"
				log.Logger.Errorw(cr.reason, "error", cr.err)
				return cr
			}
			gpuID.MinorID = strconv.Itoa(minorID)
		}

		cr.GPUIDs = append(cr.GPUIDs, gpuID)
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("all %d GPU(s) were checked", len(devs))

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	Driver Driver `json:"driver"`
	CUDA   CUDA   `json:"cuda"`

	GPUCount GPUCount `json:"gpu_count"`
	GPUIDs   []GPUID  `json:"gpu_ids"`

	Memory  Memory  `json:"memory"`
	Product Product `json:"products"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

// Driver is the driver version of the NVIDIA GPU.
type Driver struct {
	Version string `json:"version"`
}

// CUDA is the CUDA version of the NVIDIA GPU.
type CUDA struct {
	Version string `json:"version"`
}

// GPUCount is the GPUCount information of the NVIDIA GPUCount.
type GPUCount struct {
	// DeviceCount is the number of GPU devices based on the /dev directory.
	DeviceCount int `json:"device_count"`

	// Attached is the number of GPU devices that are attached to the system,
	// based on the nvidia-smi or NVML.
	Attached int `json:"attached"`
}

// GPUID represents the unique identifier of a GPU.
type GPUID struct {
	UUID    string `json:"uuid,omitempty"`
	SN      string `json:"sn,omitempty"`
	MinorID string `json:"minor_id,omitempty"`
}

// Memory is the memory information of the NVIDIA GPU.
type Memory struct {
	TotalBytes     uint64 `json:"total_bytes"`
	TotalHumanized string `json:"total_humanized"`
}

// Product is the product information of the NVIDIA GPU.
type Product struct {
	Name         string `json:"name"`
	Brand        string `json:"brand"`
	Architecture string `json:"architecture"`
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Product", cr.Product.Name})
	table.Append([]string{"Brand", cr.Product.Brand})
	table.Append([]string{"Architecture", cr.Product.Architecture})
	table.Append([]string{"Driver Version", cr.Driver.Version})
	table.Append([]string{"CUDA Version", cr.CUDA.Version})
	table.Append([]string{"GPU Count", fmt.Sprintf("%d", cr.GPUCount.DeviceCount)})
	table.Append([]string{"GPU Attached", fmt.Sprintf("%d", cr.GPUCount.Attached)})
	table.Append([]string{"GPU Memory", cr.Memory.TotalHumanized})
	table.Render()

	return buf.String()
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:      metav1.NewTime(cr.ts),
		Component: Name,
		Name:      Name,
		Reason:    cr.reason,
		Error:     cr.getError(),
		Health:    cr.health,
	}

	b, _ := json.Marshal(cr)
	state.ExtraInfo = map[string]string{"data": string(b)}
	return apiv1.HealthStates{state}
}
