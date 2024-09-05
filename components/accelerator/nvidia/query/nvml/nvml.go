// Package nvml implements the NVIDIA Management Library (NVML) interface.
// See https://docs.nvidia.com/deploy/nvml-api/nvml-api-reference.html#nvml-api-reference for more details.
package nvml

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/leptonai/gpud/log"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Output struct {
	Exists      bool          `json:"exists"`
	Message     string        `json:"message"`
	DeviceInfos []*DeviceInfo `json:"device_infos"`
}

type Instance interface {
	NVMLExists() bool

	Start() error

	XidErrorSupported() bool
	RecvXidEvents() <-chan *XidEvent

	GPMMetricsSupported() bool
	RecvGPMEvents() <-chan *GPMEvent

	Shutdown() error
	Get() (*Output, error)
}

var _ Instance = (*instance)(nil)

type instance struct {
	mu sync.RWMutex

	rootCtx    context.Context
	rootCancel context.CancelFunc

	nvmlExists    bool
	nvmlExistsMsg string

	nvmlLib   nvml.Interface
	deviceLib device.Interface
	infoLib   nvinfo.Interface

	// maps from uuid to device info
	devices map[string]*DeviceInfo

	xidErrorSupported   bool
	xidEventMask        uint64
	xidEventSet         nvml.EventSet
	xidEventCh          chan *XidEvent
	xidEventChCloseOnce sync.Once

	gpmPollInterval time.Duration

	gpmMetricsSupported bool
	gpmMetricsIDs       []nvml.GpmMetricId
	gpmEventCh          chan *GPMEvent
	gpmEventChCloseOnce sync.Once
}

type DeviceInfo struct {
	// Note that k8s-device-plugin has a different logic for MIG devices.
	// TODO: implement MIG device UUID fetching using NVML.
	UUID string `json:"uuid"`

	// MinorNumber is the minor number of the device.
	MinorNumber int `json:"minor_number"`
	// Bus is the bus ID from PCI info API.
	Bus uint32 `json:"bus"`
	// Device ID is the device ID from PCI info API.
	Device uint32 `json:"device"`

	Name            string `json:"name"`
	GPUCores        int    `json:"gpu_cores"`
	SupportedEvents uint64 `json:"supported_events"`

	// Set true if the device supports NVML error checks (health checks).
	XidErrorSupported bool `json:"xid_error_supported"`
	// Set true if the device supports GPM metrics.
	GPMMetricsSupported bool `json:"gpm_metrics_supported"`

	ClockEvents ClockEvents `json:"clock_events"`
	ClockSpeed  ClockSpeed  `json:"clock_speed"`
	Memory      Memory      `json:"memory"`
	NVLink      NVLink      `json:"nvlink"`
	Power       Power       `json:"power"`
	Temperature Temperature `json:"temperature"`
	Utilization Utilization `json:"utilization"`
	Processes   Processes   `json:"processes"`
	ECCErrors   ECCErrors   `json:"ecc_errors"`

	device device.Device `json:"-"`
}

func NewInstance(ctx context.Context, opts ...OpOption) (Instance, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}
	gpmMetricsIDs := make([]nvml.GpmMetricId, 0, len(op.gpmMetricsIDs))
	for id := range op.gpmMetricsIDs {
		gpmMetricsIDs = append(gpmMetricsIDs, id)
	}

	nvmlLib := nvml.New()
	if ret := nvmlLib.Init(); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}
	log.Logger.Debugw("successfully initialized NVML")

	deviceLib := device.New(nvmlLib)
	infoLib := nvinfo.New(
		nvinfo.WithNvmlLib(nvmlLib),
		nvinfo.WithDeviceLib(deviceLib),
	)

	nvmlExists, nvmlExistsMsg := infoLib.HasNvml()
	if !nvmlExists {
		log.Logger.Warnw("nvml not found", "message", nvmlExistsMsg)
	}

	xidEventSet, ret := nvmlLib.EventSetCreate()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to create event set: %v", nvml.ErrorString(ret))
	}

	rootCtx, rootCancel := context.WithCancel(ctx)
	return &instance{
		rootCtx:    rootCtx,
		rootCancel: rootCancel,

		nvmlLib:   nvmlLib,
		deviceLib: deviceLib,
		infoLib:   infoLib,

		nvmlExists:    nvmlExists,
		nvmlExistsMsg: nvmlExistsMsg,

		xidErrorSupported:   false,
		xidEventSet:         xidEventSet,
		xidEventMask:        defaultXidEventMask,
		xidEventCh:          make(chan *XidEvent, 100),
		xidEventChCloseOnce: sync.Once{},

		gpmPollInterval: time.Minute,

		gpmMetricsSupported: false,
		gpmMetricsIDs:       gpmMetricsIDs,
		gpmEventCh:          make(chan *GPMEvent, 100),
		gpmEventChCloseOnce: sync.Once{},
	}, nil
}

// Starts an NVML instance and starts polling for XID events.
// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/main/internal/rm/health.go
func (inst *instance) Start() error {
	if inst.nvmlLib == nil {
		return errors.New("nvml not initialized")
	}
	if !inst.nvmlExists {
		return errors.New("nvml not found")
	}

	inst.mu.Lock()
	defer inst.mu.Unlock()

	devices, err := inst.deviceLib.GetDevices()
	if err != nil {
		return err
	}

	inst.xidErrorSupported = true
	inst.gpmMetricsSupported = true

	inst.devices = make(map[string]*DeviceInfo)
	for _, d := range devices {
		uuid, ret := d.GetUUID()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device uuid: %v", nvml.ErrorString(ret))
		}
		if uuid == "" {
			return errors.New("device uuid is empty")
		}

		// TODO: this returns 0 for all GPUs...
		minorNumber, ret := d.GetMinorNumber()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device minor number: %v", nvml.ErrorString(ret))
		}

		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g8789a616b502a78a1013c45cbb86e1bd
		pciInfo, ret := d.GetPciInfo()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device PCI info: %v", nvml.ErrorString(ret))
		}

		name, ret := d.GetName()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
		}
		cores, ret := d.GetNumGpuCores()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device cores: %v", nvml.ErrorString(ret))
		}
		supportedEvents, ret := d.GetSupportedEventTypes()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get supported event types: %v", nvml.ErrorString(ret))
		}

		ret = d.RegisterEvents(inst.xidEventMask&supportedEvents, inst.xidEventSet)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to register events: %v", nvml.ErrorString(ret))
		}
		xidErrorSupported := ret != nvml.ERROR_NOT_SUPPORTED
		if !xidErrorSupported {
			inst.xidErrorSupported = false
		}

		gpmMetricsSpported, err := GPMSupportedByDevice(d)
		if err != nil {
			return err
		}
		if !gpmMetricsSpported {
			inst.gpmMetricsSupported = false
		}

		inst.devices[uuid] = &DeviceInfo{
			UUID: uuid,

			MinorNumber:     minorNumber,
			Bus:             pciInfo.Bus,
			Device:          pciInfo.Device,
			Name:            name,
			GPUCores:        cores,
			SupportedEvents: supportedEvents,

			XidErrorSupported:   xidErrorSupported,
			GPMMetricsSupported: gpmMetricsSpported,

			device: d,
		}
	}

	if inst.xidErrorSupported {
		go inst.pollXidEvents()
	} else {
		inst.xidEventChCloseOnce.Do(func() {
			log.Logger.Warnw("xid error not supported")
			close(inst.xidEventCh)
		})
	}

	if inst.gpmMetricsSupported && len(inst.gpmMetricsIDs) > 0 {
		go inst.pollGPMEvents()
	} else {
		inst.gpmEventChCloseOnce.Do(func() {
			log.Logger.Warnw("gpm metrics not supported")
			close(inst.gpmEventCh)
		})
	}

	return nil
}

func (inst *instance) NVMLExists() bool {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	return inst.nvmlLib != nil && inst.nvmlExists
}

func (inst *instance) Shutdown() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.nvmlLib == nil {
		return nil
	}

	log.Logger.Debugw("shutting down NVML")
	inst.rootCancel()

	if inst.xidEventSet != nil {
		ret := inst.xidEventSet.Free()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to free event set: %v", nvml.ErrorString(ret))
		}
	}
	inst.xidEventSet = nil

	ret := inst.nvmlLib.Shutdown()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to shutdown NVML: %v", nvml.ErrorString(ret))
	}
	inst.nvmlLib = nil

	return nil
}

// Queries the latest device info such as memory, power, temperature, etc.,
// and returns the state.
// If error happens, returns whatever queried successfully and the error.
func (inst *instance) Get() (*Output, error) {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	if inst.nvmlLib == nil {
		return nil, errors.New("nvml not initialized")
	}

	st := &Output{
		Exists:  inst.nvmlExists,
		Message: inst.nvmlExistsMsg,
	}

	for _, devInfo := range inst.devices {
		// prepare/copy the static device info
		latestInfo := &DeviceInfo{
			UUID: devInfo.UUID,

			MinorNumber: devInfo.MinorNumber,
			Bus:         devInfo.Bus,
			Device:      devInfo.Device,

			Name:            devInfo.Name,
			GPUCores:        devInfo.GPUCores,
			SupportedEvents: devInfo.SupportedEvents,

			XidErrorSupported:   devInfo.XidErrorSupported,
			GPMMetricsSupported: devInfo.GPMMetricsSupported,

			device: devInfo.device,
		}
		st.DeviceInfos = append(st.DeviceInfos, latestInfo)

		var err error
		latestInfo.ClockEvents, err = GetClockEvents(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}

		latestInfo.ClockSpeed, err = GetClockSpeed(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}

		latestInfo.Memory, err = GetMemory(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}

		latestInfo.NVLink, err = GetNVLink(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}

		latestInfo.Power, err = GetPower(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}

		latestInfo.Temperature, err = GetTemperature(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}

		latestInfo.Utilization, err = GetUtilization(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}

		latestInfo.Processes, err = GetProcesses(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}

		latestInfo.ECCErrors, err = GetECCErrors(devInfo.UUID, devInfo.device)
		if err != nil {
			return st, err
		}
	}

	sort.Slice(st.DeviceInfos, func(i, j int) bool {
		return st.DeviceInfos[i].UUID < st.DeviceInfos[j].UUID
	})

	return st, nil
}

var (
	defaultInstanceMu sync.RWMutex
	defaultInstance   Instance

	defaultInstanceReadyCloseOnce sync.Once
	defaultInstanceReadyc         = make(chan any)
)

// Starts the default NVML instance.
//
// By default, it tracks the SM occupancy metric, with nvml.GPM_METRIC_SM_OCCUPANCY.
// NVML_GPM_METRIC_SM_OCCUPANCY is the percentage of warps that were active vs theoretical maximum (0.0 - 100.0).
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlGpmStructs.html#group__nvmlGpmStructs_1g168f5f2704ec9871110d22aa1879aec0
func StartDefaultInstance(ctx context.Context) error {
	defaultInstanceMu.Lock()
	defer defaultInstanceMu.Unlock()

	if defaultInstance != nil {
		return nil
	}

	var err error
	defaultInstance, err = NewInstance(ctx, WithGPMMetricsID(nvml.GPM_METRIC_SM_OCCUPANCY))
	if err != nil {
		return err
	}

	defer func() {
		defaultInstanceReadyCloseOnce.Do(func() {
			close(defaultInstanceReadyc)
		})
	}()
	return defaultInstance.Start()
}

func DefaultInstance() Instance {
	defaultInstanceMu.RLock()
	defer defaultInstanceMu.RUnlock()

	return defaultInstance
}

func DefaultInstanceReady() <-chan any {
	defaultInstanceMu.RLock()
	defer defaultInstanceMu.RUnlock()

	return defaultInstanceReadyc
}
