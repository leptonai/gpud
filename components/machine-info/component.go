// Package machineinfo provides static information about the host (e.g., labels, IDs).
package machineinfo

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/file"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/memory"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
	nvidiaquery "github.com/leptonai/gpud/pkg/nvidia-query"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/uptime"
	"github.com/leptonai/gpud/version"
)

const Name = "machine-info"

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	annotations map[string]string
	dbRO        *sql.DB
	gatherer    prometheus.Gatherer

	nvmlInstance          nvidianvml.Instance
	getGPUDeviceCountFunc func() (int, error)
	getGPUMemoryFunc      func(uuid string, dev device.Device) (nvidianvml.Memory, error)
	getGPUSerialFunc      func(uuid string, dev device.Device) (string, error)
	getGPUMinorIDFunc     func(uuid string, dev device.Device) (int, error)

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		annotations: gpudInstance.Annotations,
		dbRO:        gpudInstance.DBRO,
		gatherer:    pkgmetrics.DefaultGatherer(),

		nvmlInstance:          gpudInstance.NVMLInstance,
		getGPUDeviceCountFunc: nvidiaquery.CountAllDevicesFromDevDir,
		getGPUMemoryFunc:      nvidianvml.GetMemory,
		getGPUSerialFunc:      nvidianvml.GetSerial,
		getGPUMinorIDFunc:     nvidianvml.GetMinorID,
	}
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		Name,
	}
}

func (c *component) IsSupported() bool {
	return true
}

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

func (c *component) checkMacAddress(cr *checkResult) error {
	interfaces, err := net.Interfaces()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting interfaces"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return err
	}

	for _, iface := range interfaces {
		macAddress := iface.HardwareAddr.String()
		if macAddress != "" {
			cr.MacAddress = macAddress
			break
		}
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	cr := &checkResult{
		DaemonVersion: version.Version,
		Annotations:   c.annotations,
		ts:            time.Now().UTC(),
	}
	defer func() {
		c.lastMu.Lock()
		c.lastCheckResult = cr
		c.lastMu.Unlock()
	}()

	if err := c.checkMacAddress(cr); err != nil {
		return cr
	}
	if err := c.checkPackages(cr); err != nil {
		return cr
	}
	if err := c.checkGPUdInfo(cr); err != nil {
		return cr
	}
	if err := c.checkGPUInfo(cr); err != nil {
		return cr
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("daemon version: %s, mac address: %s", version.Version, cr.MacAddress)

	return cr
}

func (c *component) checkPackages(cr *checkResult) error {
	if gpudmanager.GlobalController == nil {
		return nil
	}

	cctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	cr.Packages, cr.err = gpudmanager.GlobalController.Status(cctx)
	cancel()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting package status"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr.err
	}

	return nil
}

var (
	lastSQLiteMetricsMu sync.Mutex
	lastSQLiteMetrics   sqlite.Metrics
)

func (c *component) checkGPUdInfo(cr *checkResult) error {
	cr.GPUdInfo = &GPUdInfo{
		PID: os.Getpid(),
	}
	cr.GPUdInfo.UsageFileDescriptors, cr.err = file.GetCurrentProcessUsage()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting current process usage"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr.err
	}

	cr.GPUdInfo.UsageMemoryInBytes, cr.err = memory.GetCurrentProcessRSSInBytes()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting current process RSS"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr.err
	}
	cr.GPUdInfo.UsageMemoryHumanized = humanize.Bytes(cr.GPUdInfo.UsageMemoryInBytes)

	if c.dbRO != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 10*time.Second)
		cr.GPUdInfo.UsageDBInBytes, cr.err = sqlite.ReadDBSize(cctx, c.dbRO)
		ccancel()
		if cr.err != nil {
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting DB size"
			log.Logger.Errorw(cr.reason, "error", cr.err)
			return cr.err
		}
		cr.GPUdInfo.UsageDBHumanized = humanize.Bytes(cr.GPUdInfo.UsageDBInBytes)
	}

	var curMetrics sqlite.Metrics
	curMetrics, cr.err = sqlite.ReadMetrics(c.gatherer)
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting SQLite metrics"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr.err
	}

	cr.GPUdInfo.UsageInsertUpdateTotal = curMetrics.InsertUpdateTotal
	cr.GPUdInfo.UsageInsertUpdateAvgLatencyInSeconds = curMetrics.InsertUpdateSecondsAvg

	cr.GPUdInfo.UsageDeleteTotal = curMetrics.DeleteTotal
	cr.GPUdInfo.UsageDeleteAvgLatencyInSeconds = curMetrics.DeleteSecondsAvg

	cr.GPUdInfo.UsageSelectTotal = curMetrics.SelectTotal
	cr.GPUdInfo.UsageSelectAvgLatencyInSeconds = curMetrics.SelectSecondsAvg

	lastSQLiteMetricsMu.Lock()
	cr.GPUdInfo.UsageInsertUpdateAvgQPS, cr.GPUdInfo.UsageDeleteAvgQPS, cr.GPUdInfo.UsageSelectAvgQPS = lastSQLiteMetrics.QPS(curMetrics)
	lastSQLiteMetrics = curMetrics
	lastSQLiteMetricsMu.Unlock()

	cr.GPUdInfo.StartTimeInUnixTime, cr.err = uptime.GetCurrentProcessStartTimeInUnixTime()
	if cr.err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting GPUd start time"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return cr.err
	}
	cr.GPUdInfo.StartTimeHumanized = humanize.Time(time.Unix(int64(cr.GPUdInfo.StartTimeInUnixTime), 0))

	return nil
}

func (c *component) checkGPUInfo(cr *checkResult) error {
	if c.nvmlInstance == nil {
		return nil
	}
	if !c.nvmlInstance.NVMLExists() {
		return nil
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return nil
	}

	cr.GPUInfo = &GPUInfo{
		Product: GPUProduct{
			Name:         c.nvmlInstance.ProductName(),
			Brand:        c.nvmlInstance.Brand(),
			Architecture: c.nvmlInstance.Architecture(),
		},
		Driver: GPUDriver{
			Version: c.nvmlInstance.DriverVersion(),
		},
		CUDA: CUDA{
			Version: c.nvmlInstance.CUDAVersion(),
		},
	}

	deviceCount, err := c.getGPUDeviceCountFunc()
	if err != nil {
		cr.err = err
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "error getting GPU device count"
		log.Logger.Errorw(cr.reason, "error", cr.err)
		return err
	}
	cr.GPUInfo.GPUCount.DeviceCount = deviceCount

	devs := c.nvmlInstance.Devices()
	cr.GPUInfo.GPUCount.Attached = len(devs)

	for uuid, dev := range devs {
		if cr.GPUInfo.Memory.TotalBytes == 0 {
			mem, err := c.getGPUMemoryFunc(uuid, dev)
			if err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error getting memory"
				log.Logger.Errorw(cr.reason, "error", cr.err)
				return err
			}
			cr.GPUInfo.Memory.TotalBytes = mem.TotalBytes
			cr.GPUInfo.Memory.TotalHumanized = mem.TotalHumanized
		}

		gpuID := GPUID{
			UUID: uuid,
		}

		if c.getGPUSerialFunc != nil {
			serialID, err := c.getGPUSerialFunc(uuid, dev)
			if err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error getting serial id"
				log.Logger.Errorw(cr.reason, "error", cr.err)
				return err
			}
			gpuID.SN = serialID
		}

		if c.getGPUMinorIDFunc != nil {
			minorID, err := c.getGPUMinorIDFunc(uuid, dev)
			if err != nil {
				cr.err = err
				cr.health = apiv1.HealthStateTypeUnhealthy
				cr.reason = "error getting minor id"
				log.Logger.Errorw(cr.reason, "error", cr.err)
				return err
			}
			gpuID.MinorID = strconv.Itoa(minorID)
		}

		cr.GPUInfo.GPUIDs = append(cr.GPUInfo.GPUIDs, gpuID)
	}

	return nil
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// Daemon information
	DaemonVersion string                   `json:"daemon_version"`
	MacAddress    string                   `json:"mac_address"`
	Packages      []packages.PackageStatus `json:"packages"`

	// GPUdInfo represents the information about the GPUd process.
	GPUdInfo *GPUdInfo `json:"gpud_info,omitempty"`
	// GPUInfo represents the information about the GPUs (if available).
	GPUInfo *GPUInfo `json:"gpu_info,omitempty"`

	// Annotations
	Annotations map[string]string `json:"annotations"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"Daemon Version", cr.DaemonVersion})
	table.Append([]string{"Mac Address", cr.MacAddress})
	out := buf.String()

	if cr.GPUdInfo != nil {
		buf.Reset()
		cr.GPUdInfo.RenderTable(buf)
		out += "\n" + buf.String()
	}

	if cr.GPUInfo != nil {
		buf.Reset()
		cr.GPUInfo.RenderTable(buf)
		out += "\n" + buf.String()
	}

	return out
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
