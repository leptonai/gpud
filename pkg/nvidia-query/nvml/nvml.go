// Package nvml implements the NVIDIA Management Library (NVML) interface.
// See https://docs.nvidia.com/deploy/nvml-api/nvml-api-reference.html#nvml-api-reference for more details.
package nvml

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	nvml_lib "github.com/leptonai/gpud/pkg/nvidia-query/nvml/lib"
)

var _ Instance = &instance{}

type instance struct {
	mu sync.RWMutex

	driverVersion string
	cudaVersion   string

	rootCtx    context.Context
	rootCancel context.CancelFunc

	nvmlExists    bool
	nvmlExistsMsg string

	nvmlLib   nvml.Interface
	deviceLib device.Interface
	infoLib   nvinfo.Interface

	// maps from uuid to device info
	devices map[string]*DeviceInfo

	// writable database instance
	dbRW *sql.DB
	// read-only database instance
	dbRO *sql.DB

	hwSlowdownEventBucket eventstore.Bucket

	clockEventsSupported bool

	gpmPollInterval time.Duration

	gpmMetricsSupported bool
	gpmMetricsIDs       []nvml.GpmMetricId
	gpmEventCh          chan *GPMEvent
	gpmEventChCloseOnce sync.Once
}

type Instance interface {
	NVMLExists() bool

	Start() error

	ClockEventsSupported() bool

	GPMMetricsSupported() bool
	RecvGPMEvents() <-chan *GPMEvent

	Shutdown() error
	Get() (*Output, error)
}

func NewInstance(ctx context.Context, opts ...OpOption) (Instance, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	nvmlLib := nvml_lib.NewDefault()
	if installed, err := initAndCheckNVMLSupported(nvmlLib.NVML()); !installed || err != nil {
		return nil, err
	}

	log.Logger.Infow("getting driver version from nvml library")
	driverVersion, err := getDriverVersion(nvmlLib.NVML())
	if err != nil {
		return nil, err
	}
	log.Logger.Infow("successfully initialized NVML", "driverVersion", driverVersion)

	major, _, _, err := ParseDriverVersion(driverVersion)
	if err != nil {
		return nil, err
	}

	log.Logger.Infow("checking if clock events are supported")
	clockEventsSupported := ClockEventsSupportedVersion(major)
	if !clockEventsSupported {
		log.Logger.Warnw("old nvidia driver -- skipping clock events, see https://github.com/NVIDIA/go-nvml/pull/123", "version", driverVersion)
	}

	cudaVersion, err := getCUDAVersion(nvmlLib.NVML())
	if err != nil {
		return nil, err
	}

	log.Logger.Infow("checking if nvml exists from info library")
	nvmlExists, nvmlExistsMsg := nvmlLib.Info().HasNvml()
	if !nvmlExists {
		log.Logger.Warnw("nvml not found", "message", nvmlExistsMsg)
	}

	gpmMetricsIDs := make([]nvml.GpmMetricId, 0, len(op.gpmMetricsIDs))
	for id := range op.gpmMetricsIDs {
		gpmMetricsIDs = append(gpmMetricsIDs, id)
	}

	rootCtx, rootCancel := context.WithCancel(ctx)
	return &instance{
		rootCtx:    rootCtx,
		rootCancel: rootCancel,

		driverVersion: driverVersion,
		cudaVersion:   cudaVersion,

		nvmlLib:   nvmlLib.NVML(),
		deviceLib: nvmlLib.Device(),
		infoLib:   nvmlLib.Info(),

		nvmlExists:    nvmlExists,
		nvmlExistsMsg: nvmlExistsMsg,

		dbRW: op.dbRW,
		dbRO: op.dbRO,

		hwSlowdownEventBucket: op.hwSlowdownEventBucket,

		clockEventsSupported: clockEventsSupported,

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

	// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
	// "error getting device handle for index '6': Unknown Error"
	log.Logger.Debugw("getting devices from device library")
	devices, err := inst.deviceLib.GetDevices()
	if err != nil {
		return err
	}

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
		log.Logger.Debugw("getting device minor number")
		minorNumber, ret := d.GetMinorNumber()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device minor number: %v", nvml.ErrorString(ret))
		}

		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g8789a616b502a78a1013c45cbb86e1bd
		log.Logger.Debugw("getting device pci info")
		pciInfo, ret := d.GetPciInfo()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device PCI info: %v", nvml.ErrorString(ret))
		}

		log.Logger.Debugw("getting device name")
		name, ret := d.GetName()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
		}

		log.Logger.Debugw("getting device cores")
		cores, ret := d.GetNumGpuCores()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get device cores: %v", nvml.ErrorString(ret))
		}

		log.Logger.Debugw("getting supported event types")
		supportedEvents, ret := d.GetSupportedEventTypes()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to get supported event types: %v", nvml.ErrorString(ret))
		}

		log.Logger.Debugw("checking if gpm metrics are supported")
		gpmMetricsSpported, err := GPMSupportedByDevice(d)
		if err != nil {
			return err
		}
		if !gpmMetricsSpported {
			inst.gpmMetricsSupported = false
		}

		inst.devices[uuid] = &DeviceInfo{
			UUID: uuid,

			MinorNumberID: minorNumber,
			BusID:         pciInfo.Bus,
			DeviceID:      pciInfo.Device,

			Name:     name,
			GPUCores: cores,

			SupportedEvents: supportedEvents,

			GPMMetricsSupported: gpmMetricsSpported,

			device: d,
		}
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

func (inst *instance) ClockEventsSupported() bool {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	return inst.clockEventsSupported
}

func (inst *instance) Shutdown() error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.nvmlLib == nil {
		return nil
	}

	log.Logger.Debugw("shutting down NVML")
	inst.rootCancel()

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
		Exists:        inst.nvmlExists,
		Message:       inst.nvmlExistsMsg,
		DriverVersion: inst.driverVersion,
		CUDAVersion:   inst.cudaVersion,
	}

	// nvidia-smi polling happens periodically
	// so we truncate the timestamp to the nearest minute
	truncNowUTC := time.Now().UTC().Truncate(time.Minute)

	joinedErrs := make([]error, 0)
	for _, devInfo := range inst.devices {
		// prepare/copy the static device info
		latestInfo := &DeviceInfo{
			UUID: devInfo.UUID,

			MinorNumberID: devInfo.MinorNumberID,
			BusID:         devInfo.BusID,
			DeviceID:      devInfo.DeviceID,

			Name:            devInfo.Name,
			GPUCores:        devInfo.GPUCores,
			SupportedEvents: devInfo.SupportedEvents,

			GPMMetricsSupported: devInfo.GPMMetricsSupported,

			device: devInfo.device,
		}
		st.DeviceInfos = append(st.DeviceInfos, latestInfo)

		var err error
		latestInfo.GSPFirmwareMode, err = GetGSPFirmwareMode(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.PersistenceMode, err = GetPersistenceMode(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		if inst.clockEventsSupported {
			clockEvents, err := GetClockEvents(devInfo.UUID, devInfo.device)
			if err != nil {
				joinedErrs = append(joinedErrs, fmt.Errorf("failed to get clock events: %w (GPU uuid %s)", err, devInfo.UUID))
			} else {
				if len(clockEvents.HWSlowdownReasons) > 0 {
					log.Logger.Infow("detected hw slowdown clock events", "gpu_uuid", devInfo.UUID, "reasons", clockEvents.HWSlowdownReasons)

					// overwrite timestamp to the nearest minute
					clockEvents.Time = metav1.Time{Time: truncNowUTC}

					latestInfo.ClockEvents = &clockEvents

					ev := clockEvents.Event()
					if ev != nil {
						log.Logger.Infow("inserting clock events to db", "gpu_uuid", devInfo.UUID)

						cctx, ccancel := context.WithTimeout(context.Background(), 15*time.Second)
						found, err := inst.hwSlowdownEventBucket.Find(cctx, *ev)
						ccancel()
						if err != nil {
							log.Logger.Warnw("failed to find clock events from db", "error", err, "gpu_uuid", devInfo.UUID)
							joinedErrs = append(joinedErrs, fmt.Errorf("failed to find clock events: %w (GPU uuid %s)", err, devInfo.UUID))
						} else if found == nil {
							cctx, ccancel = context.WithTimeout(context.Background(), 15*time.Second)
							err = inst.hwSlowdownEventBucket.Insert(cctx, *ev)
							ccancel()
							if err != nil {
								log.Logger.Warnw("failed to insert clock events to db", "error", err, "gpu_uuid", devInfo.UUID)
							} else {
								log.Logger.Infow("inserted clock events to db", "gpu_uuid", devInfo.UUID)
							}
						} else {
							log.Logger.Infow("clock event already found in db", "gpu_uuid", devInfo.UUID)
						}
					}
				}
			}
		}

		latestInfo.ClockSpeed, err = GetClockSpeed(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.Memory, err = GetMemory(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.NVLink, err = GetNVLink(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.Power, err = GetPower(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.Temperature, err = GetTemperature(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.Utilization, err = GetUtilization(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.Processes, err = GetProcesses(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.ECCMode, err = GetECCModeEnabled(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.ECCErrors, err = GetECCErrors(devInfo.UUID, devInfo.device, latestInfo.ECCMode.EnabledCurrent)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}

		latestInfo.RemappedRows, err = GetRemappedRows(devInfo.UUID, devInfo.device)
		if err != nil {
			joinedErrs = append(joinedErrs, fmt.Errorf("%w (GPU uuid %s)", err, devInfo.UUID))
		}
	}

	sort.Slice(st.DeviceInfos, func(i, j int) bool {
		return st.DeviceInfos[i].UUID < st.DeviceInfos[j].UUID
	})

	var joinedErr error
	if len(joinedErrs) > 0 {
		joinedErr = errors.Join(joinedErrs...)
	}
	return st, joinedErr
}

func initAndCheckNVMLSupported(nvmlLib nvml.Interface) (bool, error) {
	log.Logger.Infow("initializing nvml library")
	ret := nvmlLib.Init()
	if ret == nvml.SUCCESS {
		return true, nil
	}
	if ret == nvml.ERROR_LIBRARY_NOT_FOUND {
		return false, nil
	}
	return false, fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
}

type Output struct {
	Exists  bool   `json:"exists"`
	Message string `json:"message"`

	DriverVersion string `json:"driver_version"`
	CUDAVersion   string `json:"cuda_version"`

	DeviceInfos []*DeviceInfo `json:"device_infos"`
}

// TODO
// Track if the device is a fabric-attached GPU.
// On Hopper + NVSwitch systems, GPU is registered with the NVIDIA Fabric Manager.
// Upon successful registration, the GPU is added to the NVLink fabric to enable peer-to-peer communication.
// This API reports the current state of the GPU in the NVLink fabric along with other useful information.
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g8be35e477d73cd616e57f8ad02e34154
// ref. https://github.com/NVIDIA/k8s-dra-driver/issues/2#issuecomment-2346638506
// ref. https://github.com/NVIDIA/go-nvlib/pull/44

type DeviceInfo struct {
	// Note that k8s-device-plugin has a different logic for MIG devices.
	// TODO: implement MIG device UUID fetching using NVML.
	UUID string `json:"uuid"`

	// MinorNumberID is the minor number ID of the device.
	MinorNumberID int `json:"minor_number_id"`
	// BusID is the bus ID from PCI info API.
	BusID uint32 `json:"bus_id"`
	// DeviceID is the device ID from PCI info API.
	DeviceID uint32 `json:"device_id"`

	Name            string `json:"name"`
	GPUCores        int    `json:"gpu_cores"`
	SupportedEvents uint64 `json:"supported_events"`

	// Set true if the device supports GPM metrics.
	GPMMetricsSupported bool `json:"gpm_metrics_supported"`

	GSPFirmwareMode GSPFirmwareMode `json:"gsp_firmware_mode"`
	PersistenceMode PersistenceMode `json:"persistence_mode"`
	ClockEvents     *ClockEvents    `json:"clock_events,omitempty"`
	ClockSpeed      ClockSpeed      `json:"clock_speed"`
	Memory          Memory          `json:"memory"`
	NVLink          NVLink          `json:"nvlink"`
	Power           Power           `json:"power"`
	Temperature     Temperature     `json:"temperature"`
	Utilization     Utilization     `json:"utilization"`
	Processes       Processes       `json:"processes"`
	ECCMode         ECCMode         `json:"ecc_mode"`
	ECCErrors       ECCErrors       `json:"ecc_errors"`
	RemappedRows    RemappedRows    `json:"remapped_rows"`

	device device.Device `json:"-"`
}

// architectureNames maps architecture codes to human-readable names
// as declared in nvml/nvml.h
var architectureNames = map[uint]string{
	2:          "Kepler",
	3:          "Maxwell",
	4:          "Pascal",
	5:          "Volta",
	6:          "Turing",
	7:          "Ampere",
	8:          "Ada",
	9:          "Hopper",
	0xffffffff: "Unknown",
}

func GetArchitecture(dev device.Device) (string, error) {
	arch, ret := dev.GetArchitecture()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get device architecture: %v", nvml.ErrorString(ret))
	}

	if name, ok := architectureNames[uint(arch)]; ok {
		return name, nil
	}
	return fmt.Sprintf("UnknownArchitecture(%d)", arch), nil
}

// brandNames maps brand codes to human-readable names
// as declared in nvml/nvml.h
var brandNames = map[nvml.BrandType]string{
	nvml.BRAND_UNKNOWN:             "Unknown",
	nvml.BRAND_QUADRO:              "Quadro",
	nvml.BRAND_TESLA:               "Tesla",
	nvml.BRAND_NVS:                 "NVS",
	nvml.BRAND_GRID:                "GRID",
	nvml.BRAND_GEFORCE:             "GeForce",
	nvml.BRAND_TITAN:               "TITAN",
	nvml.BRAND_NVIDIA_VAPPS:        "NVIDIA vApps",
	nvml.BRAND_NVIDIA_VPC:          "NVIDIA Virtual PC",
	nvml.BRAND_NVIDIA_VCS:          "NVIDIA Virtual Compute Server",
	nvml.BRAND_NVIDIA_VWS:          "NVIDIA Virtual Workstation",
	nvml.BRAND_NVIDIA_CLOUD_GAMING: "NVIDIA Cloud Gaming",
	nvml.BRAND_QUADRO_RTX:          "Quadro RTX",
	nvml.BRAND_NVIDIA_RTX:          "NVIDIA RTX",
	nvml.BRAND_NVIDIA:              "NVIDIA",
	nvml.BRAND_GEFORCE_RTX:         "GeForce RTX",
	nvml.BRAND_TITAN_RTX:           "TITAN RTX",
}

func GetBrand(dev device.Device) (string, error) {
	brand, ret := dev.GetBrand()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get device brand: %v", nvml.ErrorString(ret))
	}

	if name, ok := brandNames[brand]; ok {
		return name, nil
	}
	return fmt.Sprintf("UnknownBrand(%d)", brand), nil
}

func GetDriverVersion() (string, error) {
	nvmlLib := nvml_lib.NewDefault()
	if installed, err := initAndCheckNVMLSupported(nvmlLib.NVML()); !installed || err != nil {
		return "", err
	}
	defer func() {
		_ = nvmlLib.Shutdown()
	}()

	return getDriverVersion(nvmlLib.NVML())
}

func getDriverVersion(nvmlLib nvml.Interface) (string, error) {
	ver, ret := nvmlLib.SystemGetDriverVersion()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get driver version: %v", nvml.ErrorString(ret))
	}

	// e.g.,
	// 525.85.12  == does not support clock events
	// 535.161.08 == supports clock events
	return ver, nil
}

func ParseDriverVersion(version string) (major, minor, patch int, err error) {
	splits := strings.Split(version, ".")
	if len(splits) < 2 {
		return 0, 0, 0, fmt.Errorf("failed to parse driver version (expected at least 2 parts): %v", version)
	}
	if len(splits) > 3 {
		return 0, 0, 0, fmt.Errorf("failed to parse driver version (expected at most 3 parts): %v", version)
	}

	major, err = strconv.Atoi(splits[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse driver version (major): %v", err)
	}
	minor, err = strconv.Atoi(splits[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to parse driver version (minor): %v", err)
	}
	patch = 0
	if len(splits) > 2 {
		patch, err = strconv.Atoi(splits[2])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to parse driver version (patch): %v", err)
		}
	}

	return major, minor, patch, nil
}

func GetCUDAVersion() (string, error) {
	nvmlLib := nvml_lib.NewDefault()
	if installed, err := initAndCheckNVMLSupported(nvmlLib.NVML()); !installed || err != nil {
		return "", err
	}
	defer func() {
		_ = nvmlLib.Shutdown()
	}()

	return getCUDAVersion(nvmlLib.NVML())
}

func getCUDAVersion(nvmlLib nvml.Interface) (string, error) {
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlSystemQueries.html#group__nvmlSystemQueries_1g1d12b603a42805ee7e4160557ffc2128
	ver, ret := nvmlLib.SystemGetCudaDriverVersion_v2()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get driver version: %v", nvml.ErrorString(ret))
	}

	// #define NVML_CUDA_DRIVER_VERSION_MAJOR ( v ) ((v)/1000)
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlSystemQueries.html#group__nvmlSystemQueries_1g40a4eb255d9766f6bc4c9402ce9102c2
	major := ver / 1000

	// #define NVML_CUDA_DRIVER_VERSION_MINOR ( v ) (((v) % 1000) / 10)
	minor := (ver % 1000) / 10

	return fmt.Sprintf("%d.%d", major, minor), nil
}

// clock events are supported in versions 535 and above
// otherwise, CGO call just exits with
// undefined symbol: nvmlDeviceGetCurrentClocksEventReasons
func ClockEventsSupportedVersion(major int) bool {
	return major >= 535
}

// Loads the product name of the NVIDIA GPU device.
func LoadGPUDeviceName() (string, error) {
	nvmlLib := nvml_lib.NewDefault()
	if installed, err := initAndCheckNVMLSupported(nvmlLib.NVML()); !installed || err != nil {
		return "", err
	}
	defer func() {
		_ = nvmlLib.Shutdown()
	}()

	nvmlExists, nvmlExistsMsg := nvmlLib.Info().HasNvml()
	if !nvmlExists {
		return "", fmt.Errorf("NVML not found: %s", nvmlExistsMsg)
	}

	// "NVIDIA Xid 79: GPU has fallen off the bus" may fail this syscall with:
	// "error getting device handle for index '6': Unknown Error"
	devices, err := nvmlLib.Device().GetDevices()
	if err != nil {
		return "", err
	}

	for _, d := range devices {
		name, ret := d.GetName()
		if ret != nvml.SUCCESS {
			return "", fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
		}
		if name != "" {
			return name, nil
		}
	}

	return "", nil
}

var (
	defaultInstanceMu sync.RWMutex
	defaultInstance   Instance

	defaultInstanceReadyCloseOnce sync.Once
	defaultInstanceReadyc         = make(chan any)
)

// Starts the default NVML instance.
//
// By default, it tracks the SM occupancy metrics, with nvml.GPM_METRIC_SM_OCCUPANCY,
// nvml.GPM_METRIC_INTEGER_UTIL, nvml.GPM_METRIC_ANY_TENSOR_UTIL,
// nvml.GPM_METRIC_DFMA_TENSOR_UTIL, nvml.GPM_METRIC_HMMA_TENSOR_UTIL,
// nvml.GPM_METRIC_IMMA_TENSOR_UTIL, nvml.GPM_METRIC_FP64_UTIL,
// nvml.GPM_METRIC_FP32_UTIL, nvml.GPM_METRIC_FP16_UTIL,
//
// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10641-L10643
// NVML_GPM_METRIC_SM_OCCUPANCY is the percentage of warps that were active vs theoretical maximum (0.0 - 100.0).
// NVML_GPM_METRIC_INTEGER_UTIL is the percentage of time the GPU's SMs were doing integer operations (0.0 - 100.0).
// NVML_GPM_METRIC_ANY_TENSOR_UTIL is the percentage of time the GPU's SMs were doing ANY tensor operations (0.0 - 100.0).
//
// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10644-L10646
// NVML_GPM_METRIC_DFMA_TENSOR_UTIL is the percentage of time the GPU's SMs were doing DFMA tensor operations (0.0 - 100.0).
// NVML_GPM_METRIC_HMMA_TENSOR_UTIL is the percentage of time the GPU's SMs were doing HMMA tensor operations (0.0 - 100.0).
// NVML_GPM_METRIC_IMMA_TENSOR_UTIL is the percentage of time the GPU's SMs were doing IMMA tensor operations (0.0 - 100.0).
//
// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10648-L10650
// NVML_GPM_METRIC_FP64_UTIL is the percentage of time the GPU's SMs were doing non-tensor FP64 math (0.0 - 100.0).
// NVML_GPM_METRIC_FP32_UTIL is the percentage of time the GPU's SMs were doing non-tensor FP32 math (0.0 - 100.0).
// NVML_GPM_METRIC_FP16_UTIL is the percentage of time the GPU's SMs were doing non-tensor FP16 math (0.0 - 100.0).
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlGpmStructs.html#group__nvmlGpmStructs_1g168f5f2704ec9871110d22aa1879aec0
//
// Note that the "rootCtx" is used for instantiating the "shared" NVML instance "once"
// and all other sub-calls have its own context timeouts, thus the caller should not set the timeout here.
// Otherwise, we will cancel all future operations when the instance is created only once!
func StartDefaultInstance(rootCtx context.Context, opts ...OpOption) error {
	defaultInstanceMu.Lock()
	defer defaultInstanceMu.Unlock()

	// to only create "once"!
	if defaultInstance != nil {
		return nil
	}

	log.Logger.Debugw("creating a new default nvml instance")

	var err error
	defaultInstance, err = NewInstance(rootCtx, opts...)
	if err != nil {
		return err
	}

	defer func() {
		defaultInstanceReadyCloseOnce.Do(func() {
			log.Logger.Infow("signaling that the default nvml instance is ready")
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

func GetProductName(dev device.Device) (string, error) {
	name, ret := dev.GetName()
	if ret != nvml.SUCCESS {
		return "", fmt.Errorf("failed to get device name: %v", nvml.ErrorString(ret))
	}
	return name, nil
}
