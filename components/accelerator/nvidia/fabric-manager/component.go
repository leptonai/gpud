// Package fabricmanager tracks NVIDIA fabric manager and system management services.
//
// # Fabric Management Architecture
//
// NVIDIA systems use different fabric management approaches depending on the generation:
//
// ## Pre-NVL5 Systems (DGX A100, DGX H100, HGX A100, HGX H100)
//
// Traditional nvidia-fabricmanager daemon running on compute nodes:
//   - Service: nvidia-fabricmanager.service
//   - Port: 6666 (FM_CMD_PORT_NUMBER)
//   - Architecture: Userspace daemon managing NVSwitch kernel driver
//   - Requires: /dev/nvidia-switch* devices via kernel driver
//   - Reference: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/
//
// ## NVL5+ Systems (GB200 NVL72)
//
// Distributed fabric management architecture:
//
// ### NVLink Switch Trays - Run NVOS (NVSwitch Operating System)
//
// NVOS includes integrated fabric management services:
//
//   Quote: "NVOS includes the NVLink Subnet Manager (NVLSM), the Fabric Manager (FM),
//          NMX services such as NMX-Controller and NMX-Telemetry, and the NVSwitch firmware."
//   Reference: https://docs.nvidia.com/networking/display/nvidianvosusermanualfornvlinkswitchesv25021884/cluster+management
//
//   Quote: "NVOS software image includes the NMX-C application, the FM application,
//          and the NVLSM application, with no standalone software installation required
//          for these components."
//   Reference: https://docs.nvidia.com/multi-node-nvlink-systems/mnnvl-user-guide/overview.html
//
// NMX-Controller (NMX-C) - Provides Global Fabric Manager (GFM):
//
//   Quote: "In the GB200 NVL the SDN services are the subnet manager (SM) and
//          global fabric manager (GFM)"
//   Reference: https://docs.nvidia.com/networking/display/nmxcv11/nmx-controller
//
// ### Compute Nodes - Run NVSM (NVIDIA System Management)
//
// NVSM provides system management and fabric health monitoring on compute nodes:
//   - Services: nvsm-core.service, nvsm-api-gateway.service
//   - Port: 273 (nvsm-api-gateway REST API)
//   - Function: Monitors system health and fabric state
//   - Reference: https://docs.nvidia.com/datacenter/nvsm/nvsm-user-guide/latest/
//   - Reference: https://docs.nvidia.com/dgx/dgxgb200-user-guide/software.html
//
// On GB200 compute nodes:
//   - Traditional fabric-manager daemon (port 6666) does NOT run
//   - NMX services do NOT run (they run on switch trays, not compute nodes)
//   - Fabric management is handled by NVOS on the switch trays
//   - NVSM handles system management and monitors fabric health
//
// Attempting to start traditional fabric-manager on GB200 fails with NV_WARN_NOTHING_TO_DO
// because no NVSwitch kernel driver/devices are present on compute nodes.
// Reference: https://github.com/NVIDIA/gpu-operator/issues/610
//
// # Detection Strategy
//
// This component checks for fabric management services in the following order:
//  1. Traditional fabric-manager on port 6666 (Pre-NVL5 systems)
//  2. NVSM on port 273 (GB200 NVL72 compute nodes) - fallback if port 6666 check fails
package fabricmanager

import (
	"context"
	"encoding/json"
	"os/exec"
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

	// defaultFabricManagerPort is the TCP port for traditional nvidia-fabricmanager API.
	// Used on Pre-NVL5 systems: DGX A100, DGX H100, HGX A100, HGX H100.
	// The traditional fabric-manager daemon runs on compute nodes and manages
	// NVSwitch devices via kernel driver.
	// Reference: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html#the-fabric-manager-api-tcp-port
	defaultFabricManagerPort = 6666

	// nvsmPort is the TCP port for NVSM (NVIDIA System Management) API gateway.
	// Used on GB200 NVL72 compute nodes where fabric management is integrated into
	// NVOS running on NVLink Switch Trays. NVSM provides system management and
	// fabric health monitoring on compute nodes.
	// Reference: https://docs.nvidia.com/datacenter/nvsm/nvsm-user-guide/latest/
	nvsmPort = 273
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	nvmlInstance nvidianvml.Instance

	checkNVSwitchExistsFunc func() bool

	checkFMExistsFunc   func() bool
	checkFMActiveFunc   func() bool
	checkNVSMActiveFunc func() bool

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

		checkFMExistsFunc:   checkFMExists,
		checkFMActiveFunc:   checkFMActive,
		checkNVSMActiveFunc: checkNVSMActive,
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

	// Determine which type of fabric management is active
	// Check NVSM first since it's the fallback in checkFMActive()
	if c.checkNVSMActiveFunc != nil && c.checkNVSMActiveFunc() {
		cr.FabricManagerType = "nvsm"
		cr.reason = "fabric manager found and active (NVSM on port 273)"
	} else {
		// If checkFMActiveFunc returned true but NVSM is not active,
		// it must be traditional fabric-manager on port 6666
		cr.FabricManagerType = "traditional"
		cr.reason = "fabric manager found and active (traditional on port 6666)"
	}

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

// checkNVSMActive returns true if NVSM (NVIDIA System Management) is active
// by checking if port 273 is listening (nvsm-api-gateway service).
//
// NVSM is used on GB200 NVL72 compute nodes (NVL5+ architecture) where fabric
// management is integrated into NVOS (NVSwitch Operating System) running on
// the NVLink Switch Trays.
//
// On GB200 systems:
//   - NVLink Switch Trays run NVOS with integrated Fabric Manager and NMX-Controller
//   - Compute nodes run NVSM for system management and fabric health monitoring
//   - Traditional fabric-manager daemon does NOT run on compute nodes
//
// Quote: "NVOS includes the NVLink Subnet Manager (NVLSM), the Fabric Manager (FM),
//        NMX services such as NMX-Controller and NMX-Telemetry, and the NVSwitch firmware."
// Reference: https://docs.nvidia.com/networking/display/nvidianvosusermanualfornvlinkswitchesv25021884/cluster+management
//
// Reference: https://docs.nvidia.com/datacenter/nvsm/nvsm-user-guide/latest/
func checkNVSMActive() bool {
	return netutil.IsPortOpen(nvsmPort)
}

// checkFMActive returns true if fabric management is active by checking listening ports.
//
// Detection strategy:
//  1. Check port 6666 (traditional nvidia-fabricmanager for Pre-NVL5 systems)
//  2. If port 6666 fails, fallback to port 273 (NVSM for GB200 NVL72 systems)
//
// # Traditional Fabric Manager (Port 6666) - Pre-NVL5 Systems
//
// Pre-NVL5 systems (DGX A100, DGX H100, HGX A100, HGX H100):
//   - nvidia-fabricmanager daemon runs on compute nodes
//   - Manages NVSwitch devices via kernel driver (/dev/nvidia-switch*)
//   - Listens on port 6666 for management API
//   - Reference: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/
//
// # NVSM (Port 273) - GB200 NVL72 Systems
//
// GB200 NVL72 architecture separates fabric management and system management:
//
// NVLink Switch Trays run NVOS with integrated fabric management:
//   Quote: "NVOS includes the NVLink Subnet Manager (NVLSM), the Fabric Manager (FM),
//          NMX services such as NMX-Controller and NMX-Telemetry, and the NVSwitch firmware."
//   Reference: https://docs.nvidia.com/networking/display/nvidianvosusermanualfornvlinkswitchesv25021884/cluster+management
//
//   Quote: "In the GB200 NVL the SDN services are the subnet manager (SM) and
//          global fabric manager (GFM)"
//   Reference: https://docs.nvidia.com/networking/display/nmxcv11/nmx-controller
//
// Compute Nodes run NVSM for system management:
//   - nvsm-core.service: Monitors system health and fabric state
//   - nvsm-api-gateway.service: REST API on port 273
//   - NO traditional fabric-manager daemon (port 6666)
//   - NO NMX services (those run on switch trays)
//   - Reference: https://docs.nvidia.com/datacenter/nvsm/nvsm-user-guide/latest/
//
// On GB200, attempting to start traditional fabric-manager fails with NV_WARN_NOTHING_TO_DO
// because fabric management is integrated into NVOS on switch trays, not on compute nodes.
// Reference: https://github.com/NVIDIA/gpu-operator/issues/610
func checkFMActive() bool {
	// First, check traditional fabric-manager port (Pre-NVL5: DGX A100/H100)
	if netutil.IsPortOpen(defaultFabricManagerPort) {
		return true
	}

	// Fallback: Check NVSM port (GB200 NVL72 compute nodes)
	// On GB200, fabric management is handled by NVOS on switch trays,
	// while NVSM handles system management on compute nodes.
	if netutil.IsPortOpen(nvsmPort) {
		log.Logger.Warnw(
			"traditional fabric-manager (port 6666) not detected, falling back to NVSM detection",
			"nvsmPort", nvsmPort,
			"reason", "GB200 NVL72 systems use NVSM on compute nodes; fabric management runs on NVLink Switch Trays in NVOS",
		)
		return true
	}

	return false
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	// FabricManagerActive is true if fabric management is active.
	// This includes:
	//   - Traditional fabric-manager on port 6666 (Pre-NVL5: DGX A100/H100)
	//   - NVSM on port 273 (GB200 NVL72 compute nodes)
	FabricManagerActive bool `json:"fabric_manager_active"`

	// FabricManagerType indicates which fabric management system is active.
	// Values: "traditional" (port 6666), "nvsm" (port 273), or empty if inactive.
	FabricManagerType string `json:"fabric_manager_type,omitempty"`

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
