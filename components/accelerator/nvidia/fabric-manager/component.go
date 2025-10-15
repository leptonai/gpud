// Package fabricmanager tracks NVIDIA fabric manager and fabric health monitoring services.
//
// # Fabric Management Architecture
//
// NVIDIA systems use different fabric management and monitoring approaches depending on the GPU generation:
//
// ## Pre-NVL5 Systems (DGX A100, DGX H100, HGX A100, HGX H100)
//
// Traditional nvidia-fabricmanager daemon running on compute nodes:
//   - Service: nvidia-fabricmanager.service
//   - Port: 6666 (FM_CMD_PORT_NUMBER)
//   - Architecture: Userspace daemon managing NVSwitch kernel driver
//   - Requires: /dev/nvidia-switch* devices via kernel driver
//   - Monitoring: Service activeness via port check
//   - Reference: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/
//
// ## NVL5+ Systems (GB200 NVL72)
//
// Distributed fabric management architecture with NVML-based health monitoring:
//
// ### NVLink Switch Trays - Run NVOS (NVSwitch Operating System)
//
// NVOS includes integrated fabric management services:
//
//	Quote: "NVOS includes the NVLink Subnet Manager (NVLSM), the Fabric Manager (FM),
//	       NMX services such as NMX-Controller and NMX-Telemetry, and the NVSwitch firmware."
//	Reference: https://docs.nvidia.com/networking/display/nvidianvosusermanualfornvlinkswitchesv25021884/cluster+management
//
//	Quote: "NVOS software image includes the NMX-C application, the FM application,
//	       and the NVLSM application, with no standalone software installation required
//	       for these components."
//	Reference: https://docs.nvidia.com/multi-node-nvlink-systems/mnnvl-user-guide/overview.html
//
// NMX-Controller (NMX-C) - Provides Global Fabric Manager (GFM):
//
//	Quote: "In the GB200 NVL the SDN services are the subnet manager (SM) and
//	       global fabric manager (GFM)"
//	Reference: https://docs.nvidia.com/networking/display/nmxcv11/nmx-controller
//
// ### Compute Nodes - Run NVSM (NVIDIA System Management)
//
// NVSM provides system management and exposes fabric health via NVML APIs:
//   - Services: nvsm-core.service, nvsm-api-gateway.service
//   - Port: 273 (nvsm-api-gateway REST API)
//   - Function: Monitors system health, exposes fabric state via nvmlDeviceGetGpuFabricInfo*
//   - Reference: https://docs.nvidia.com/datacenter/nvsm/nvsm-user-guide/latest/
//   - Reference: https://docs.nvidia.com/dgx/dgxgb200-user-guide/software.html
//
// On GB200 compute nodes:
//   - Traditional fabric-manager daemon (port 6666) does NOT run
//   - NMX services do NOT run (they run on switch trays, not compute nodes)
//   - Fabric management is handled by NVOS on the switch trays
//   - NVSM handles system management and exposes fabric health telemetry via NVML
//
// Attempting to start traditional fabric-manager on GB200 fails with NV_WARN_NOTHING_TO_DO
// because no NVSwitch kernel driver/devices are present on compute nodes.
// Reference: https://github.com/NVIDIA/gpu-operator/issues/610
//
// # Fabric Health Monitoring via NVML
//
// For GB200 and newer GPUs that support fabric state telemetry, this component uses:
//   - nvmlDeviceGetGpuFabricInfo() for basic fabric info (V1 API)
//   - nvmlDeviceGetGpuFabricInfoV().V3() for detailed health metrics (V3 API)
//
// The V3 API provides comprehensive health information including:
//   - Clique ID and Cluster UUID
//   - Fabric state (Not Started, In Progress, Completed)
//   - Health summary (Healthy, Unhealthy, Limited Capacity)
//   - Detailed health mask covering:
//   - Bandwidth status (Full, Degraded)
//   - Route recovery progress
//   - Route health status
//   - Access timeout recovery
//
// # Detection Strategy
//
// This component checks for fabric management/monitoring in the following order:
//  1. Check if nvmlDeviceGetGpuFabricInfo* is supported (GB200 NVL72 and newer)
//     - If supported, use NVML fabric state APIs for health monitoring
//     - This path is taken for systems with NVSM-based fabric telemetry
//  2. Check traditional fabric-manager on port 6666 (Pre-NVL5 systems)
//     - For DGX A100, DGX H100, HGX A100, HGX H100
//     - Validates service activeness and monitors logs for errors
package fabricmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/log"
	netutil "github.com/leptonai/gpud/pkg/netutil"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

const (
	Name = "accelerator-nvidia-fabric-manager"

	// defaultFabricManagerPort is the TCP port for traditional nvidia-fabricmanager API (FM_CMD_PORT_NUMBER).
	// Used on Pre-NVL5 systems: DGX A100, DGX H100, HGX A100, HGX H100.
	// The traditional fabric-manager daemon runs on compute nodes and manages
	// NVSwitch devices via kernel driver.
	// Reference: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html#the-fabric-manager-api-tcp-port
	defaultFabricManagerPort = 6666
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	collectFabricStateFunc func() fabricStateReport

	checkNVSwitchExistsFunc func() bool

	checkFMExistsFunc func() bool
	checkFMActiveFunc func() bool

	eventBucket      eventstore.Bucket
	logLineProcessor *logLineProcessor

	lastMu          sync.RWMutex
	lastCheckResult *checkResult
}

func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,

		nvmlInstance: gpudInstance.NVMLInstance,

		collectFabricStateFunc: func() fabricStateReport {
			report := fabricStateReport{Healthy: true}

			if gpudInstance.NVMLInstance == nil {
				report.Err = fmt.Errorf("nvml instance is nil")
				report.Healthy = false
				return report
			}

			devices := gpudInstance.NVMLInstance.Devices()
			if len(devices) == 0 {
				return report
			}

			uuids := make([]string, 0, len(devices))
			for uuid := range devices {
				uuids = append(uuids, uuid)
			}
			sort.Strings(uuids)

			reasons := make([]string, 0)

			for _, uuid := range uuids {
				dev := devices[uuid]
				info, err := getFabricInfo(dev)
				if err != nil {
					report.Err = fmt.Errorf("fabric state query failed for GPU %s: %w", uuid, err)
					report.Healthy = false
					return report
				}

				entry, issues := formatFabricStateEntry(uuid, info)
				report.Entries = append(report.Entries, entry)
				if len(issues) > 0 {
					report.Healthy = false
					reasons = append(reasons, fmt.Sprintf("GPU %s: %s", uuid, strings.Join(issues, ", ")))
				}
			}

			if len(reasons) > 0 {
				report.Reason = strings.Join(reasons, "; ")
			}

			return report
		},

		checkNVSwitchExistsFunc: func() bool {
			devCnt := len(gpudInstance.NVMLInstance.Devices())
			if devCnt <= 1 {
				return false
			}

			lines, err := ListPCINVSwitches(cctx)
			if err != nil {
				log.Logger.Errorw("failed to list nvidia pci switches", "error", err)

				// Fallback to nvidia-smi nvlink detection method
				lines, err = CountSMINVSwitches(cctx)
				if err != nil {
					log.Logger.Errorw("failed to count nvidia smi nvlink switches", "error", err)
					return false
				}
			}
			return len(lines) > 0
		},

		checkFMExistsFunc: checkFMExists,
		checkFMActiveFunc: checkFMActive,
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name)
		if err != nil {
			ccancel()
			return nil, err
		}
	}

	if c.checkFMExistsFunc() && c.eventBucket != nil {
		w, err := newWatcher(defaultWatchCommands)
		if err != nil {
			ccancel()
			return nil, err
		}
		c.logLineProcessor = newLogLineProcessor(cctx, w, Match, c.eventBucket)
	}

	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}
}

func (c *component) IsSupported() bool {
	if c.nvmlInstance == nil {
		return false
	}
	return c.nvmlInstance.NVMLExists() && c.nvmlInstance.ProductName() != ""
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
	if c.logLineProcessor == nil {
		return nil, nil
	}
	return c.logLineProcessor.getEvents(ctx, since)
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.logLineProcessor != nil {
		c.logLineProcessor.close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia fabric manager")

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

	if c.nvmlInstance.FabricStateSupported() {
		report := c.collectFabricStateFunc()
		cr.FabricStateSupported = true
		cr.FabricStates = report.Entries
		if report.Reason != "" {
			cr.FabricStateReason = report.Reason
		}

		// GB200 NVL72 Fabric Management Architecture
		//
		// On GB200 NVL72 systems, fabric management is handled differently than traditional systems:
		//
		// NVLink Switch Trays run NVOS (NVSwitch Operating System) which includes:
		//   - Fabric Manager (FM)
		//   - NVLink Subnet Manager (NVLSM)
		//   - NMX-Controller (provides Global Fabric Manager)
		//   - NMX-Telemetry
		//
		// Quote: "NVOS includes the NVLink Subnet Manager (NVLSM), the Fabric Manager (FM),
		//        NMX services such as NMX-Controller and NMX-Telemetry, and the NVSwitch firmware."
		// Reference: https://docs.nvidia.com/networking/display/nvidianvosusermanualfornvlinkswitchesv25021884/cluster+management
		//
		// Compute Nodes run NVSM (NVIDIA System Management) which:
		//   - Provides system management and monitoring
		//   - Exposes fabric health via nvmlDeviceGetGpuFabricInfo* APIs
		//   - Does NOT run traditional fabric-manager daemon (port 6666)
		//   - Does NOT run NMX services (those run on switch trays)
		//
		// Reference: https://docs.nvidia.com/datacenter/nvsm/nvsm-user-guide/latest/
		// Reference: https://docs.nvidia.com/dgx/dgxgb200-user-guide/software.html
		//
		// Attempting to start traditional fabric-manager on GB200 compute nodes fails with
		// NV_WARN_NOTHING_TO_DO because no NVSwitch kernel driver/devices are present.
		// Reference: https://github.com/NVIDIA/gpu-operator/issues/610
		//
		// Therefore, for GB200 and newer GPUs with fabric state support, we use NVML APIs
		// (nvmlDeviceGetGpuFabricInfo*) to monitor fabric health instead of checking for
		// the traditional fabric-manager daemon.

		// Log fabric state issues for monitoring purposes
		// TODO: set health state based on the fabric state once we determine the appropriate
		// health criteria and thresholds for production use
		if report.Err != nil {
			log.Logger.Warnw("failed to collect fabric state", "reason", report.Reason, "error", report.Err)
		}
		if !report.Healthy {
			log.Logger.Warnw("fabric state is not healthy", "reason", report.Reason)
		}

		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = c.nvmlInstance.ProductName() + " checked fabric state"
		return cr
	}

	if !c.nvmlInstance.FabricManagerSupported() {
		cr.FabricManagerActive = false
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = c.nvmlInstance.ProductName() + " does not support fabric manager"
		return cr
	}

	if c.checkNVSwitchExistsFunc != nil && !c.checkNVSwitchExistsFunc() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVSwitch not detected, skipping fabric manager check"
		return cr
	}

	if !c.checkFMExistsFunc() {
		cr.FabricManagerActive = false
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "nv-fabricmanager executable not found"
		return cr
	}

	active := c.checkFMActiveFunc()
	if !active {
		cr.FabricManagerActive = false

		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = "nv-fabricmanager found but fabric manager service is not active"

		return cr
	}

	cr.FabricManagerActive = true
	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = "fabric manager found and active"

	return cr
}

// checkFMExists returns true if the fabric manager executable is found in the system.
func checkFMExists() bool {
	p, err := exec.LookPath("nv-fabricmanager")
	if err != nil {
		return false
	}
	return p != ""
}

// checkFMActive returns true if the traditional fabric manager is active by checking its listening port.
//
// Checks port 6666 (FM_CMD_PORT_NUMBER) for the traditional nvidia-fabricmanager daemon.
// This is used on Pre-NVL5 systems (DGX A100, DGX H100, HGX A100, HGX H100) where the
// fabric-manager daemon runs on compute nodes and manages NVSwitch devices via kernel driver.
//
// For GB200 NVL72 systems, this check will fail (return false) because:
//   - Traditional fabric-manager daemon does NOT run on compute nodes
//   - Fabric management is integrated into NVOS running on NVLink Switch Trays
//   - NVSM provides fabric health monitoring via nvmlDeviceGetGpuFabricInfo* APIs
//
// In such cases, the component uses the FabricStateSupported check instead, which is
// evaluated earlier in the Check() method and bypasses this traditional fabric manager check.
//
// Reference: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html#the-fabric-manager-api-tcp-port
//
// Alternative implementation: We could check dbus connection to see if the systemd
// "nvidia-fabricmanager" service is active, but port checking is simpler and sufficient.
func checkFMActive() bool {
	return netutil.IsPortOpen(defaultFabricManagerPort)
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// FabricManagerActive is true if the fabric manager is active.
	// By default, it checks the "nv-fabricmanager" default listening port 6666.
	FabricManagerActive bool `json:"fabric_manager_active"`

	// FabricStateSupported reports whether NVML fabric state telemetry is
	// available for this GPU generation (e.g. GB200 via NVOS/NVSM).
	FabricStateSupported bool `json:"fabric_state_supported,omitempty"`
	// FabricStates captures per-GPU fabric probe results pulled from
	// nvmlDeviceGetGpuFabricInfo*.
	FabricStates []fabricStateEntry `json:"fabric_states,omitempty"`
	// FabricStateReason captures any aggregated fabric health warnings.
	FabricStateReason string `json:"fabric_state_reason,omitempty"`

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

	// If fabric state is supported and we have entries, render the fabric state table
	if cr.FabricStateSupported && len(cr.FabricStates) > 0 {
		var buf bytes.Buffer
		for i, state := range cr.FabricStates {
			if i > 0 {
				buf.WriteString("\n")
			}
			state.RenderTable(&buf)
		}
		return buf.String()
	}

	// Otherwise, fall back to simple fabric manager status
	if cr.FabricManagerActive {
		return "fabric manager is active"
	}

	return "fabric manager is not active"
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
