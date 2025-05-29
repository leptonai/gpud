package v1

import (
	"fmt"
	"io"
	"net/netip"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HealthStateType defines the health state of a component.
type HealthStateType string

const (
	HealthStateTypeHealthy      HealthStateType = "Healthy"
	HealthStateTypeUnhealthy    HealthStateType = "Unhealthy"
	HealthStateTypeDegraded     HealthStateType = "Degraded"
	HealthStateTypeInitializing HealthStateType = "Initializing"
)

// ComponentType defines the type of a component.
type ComponentType string

const (
	// ComponentTypeCustomPlugin represents a custom plugin of GPUd.
	ComponentTypeCustomPlugin ComponentType = "custom-plugin"
)

// RunModeType defines the run mode of a component.
type RunModeType string

const (
	// RunModeTypeAuto is the run mode that runs automatically with the specified interval
	// when enabled as a component.
	RunModeTypeAuto RunModeType = "auto"
	// RunModeTypeManual is the run mode that requires manual trigger to run the check.
	RunModeTypeManual RunModeType = "manual"
)

// HealthState represents the health state of a component.
// The healthiness of the component is already evaluated at the component level,
// so the health state here is to provide more details about the healthiness,
// and other data for the control plane to decide how to alert and remediate the issue.
type HealthState struct {
	// Time represents when the event happened.
	Time metav1.Time `json:"time"`

	// Component represents the component name.
	Component string `json:"component,omitempty"`
	// ComponentType represents the type of the component.
	// It is either "" (just 'component') or "custom-plugin".
	ComponentType ComponentType `json:"component_type,omitempty"`

	// Name is the name of the state,
	// can be different from the component name.
	Name string `json:"name,omitempty"`

	// RunMode is the run mode of the state.
	// It can be "manual" that requires manual trigger to run the check.
	// Or it can be empty that runs the check periodically.
	RunMode RunModeType `json:"run_mode,omitempty"`

	// Health represents the health level of the state,
	// including StateHealthy, StateUnhealthy and StateDegraded.
	// StateDegraded is similar to Unhealthy which also can trigger alerts
	// for users or operators, but what StateDegraded means is that the
	// issue detected does not affect users’ workload.
	Health HealthStateType `json:"health,omitempty"`

	// Reason represents what happened or detected by GPUd if it isn’t healthy.
	Reason string `json:"reason,omitempty"`

	// Error represents the detailed error information, which will be shown
	// as More Information to help analyze why it isn’t healthy.
	Error string `json:"error,omitempty"`

	// SuggestedActions represents the suggested actions to mitigate the issue.
	SuggestedActions *SuggestedActions `json:"suggested_actions,omitempty"`

	// ExtraInfo represents the extra information of the state.
	ExtraInfo map[string]string `json:"extra_info,omitempty"`
}

type HealthStates []HealthState

type ComponentHealthStates struct {
	Component string       `json:"component"`
	States    HealthStates `json:"states"`
}

type GPUdComponentHealthStates []ComponentHealthStates

// Event represents an event that happened in a component at a specific time.
// A single event itself does not dictate whether the component is healthy or not.
// The healthiness of the component is evaluated at the component health state level.
type Event struct {
	// Component represents which component generated the event.
	Component string `json:"component,omitempty"`

	// Time represents when the event happened.
	Time metav1.Time `json:"time"`

	// Name represents the name of the event.
	Name string `json:"name,omitempty"`

	// Type represents the type of the event.
	Type EventType `json:"type,omitempty"`

	// Message represents the detailed message of the event.
	Message string `json:"message,omitempty"`
}

type Events []Event

type ComponentEvents struct {
	Component string    `json:"component"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Events    Events    `json:"events"`
}

type GPUdComponentEvents []ComponentEvents

type Metric struct {
	UnixSeconds int64             `json:"unix_seconds"`
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Value       float64           `json:"value"`
}

type Metrics []Metric

type ComponentMetrics struct {
	Component string  `json:"component"`
	Metrics   Metrics `json:"metrics"`
}

type GPUdComponentMetrics []ComponentMetrics

type Info struct {
	States  HealthStates `json:"states"`
	Events  Events       `json:"events"`
	Metrics Metrics      `json:"metrics"`
}

type ComponentInfo struct {
	Component string    `json:"component"`
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Info      Info      `json:"info"`
}

type GPUdComponentInfos []ComponentInfo

type PackageStatus struct {
	Name           string       `json:"name"`
	Phase          PackagePhase `json:"phase"`
	Status         string       `json:"status"`
	CurrentVersion string       `json:"current_version"`
}

type PackagePhase string

const (
	InstalledPhase  PackagePhase = "Installed"
	InstallingPhase PackagePhase = "Installing"
	UnknownPhase    PackagePhase = "Unknown"
)

type RepairActionType string

const (
	// RepairActionTypeIgnoreNoActionRequired represents a suggested action to ignore the issue,
	// meaning no action is needed until further notice.
	RepairActionTypeIgnoreNoActionRequired RepairActionType = "IGNORE_NO_ACTION_REQUIRED"

	// RepairActionTypeRebootSystem represents a suggested action to reboot the system.
	// Specific to NVIDIA GPUs, this implies GPU reset by rebooting the system.
	RepairActionTypeRebootSystem RepairActionType = "REBOOT_SYSTEM"

	// RepairActionTypeHardwareInspection represents a suggested action for hardware inspection
	// and repair if any issue is found. This often involves data center (or cloud provider) support
	// to physically check/repair the machine.
	RepairActionTypeHardwareInspection RepairActionType = "HARDWARE_INSPECTION"

	// RepairActionTypeCheckUserApp represents a suggested action to check the user application.
	// For instance, NVIDIA may report XID 45 as user app error, but the underlying GPU might have other issues
	// thus requires further diagnosis of the application and the GPU.
	RepairActionTypeCheckUserAppAndGPU RepairActionType = "CHECK_USER_APP_AND_GPU"
)

// SuggestedActions represents a set of suggested actions to mitigate an issue.
type SuggestedActions struct {
	// Description describes the issue in detail.
	Description string `json:"description"`

	// A list of repair actions to mitigate the issue.
	RepairActions []RepairActionType `json:"repair_actions"`
}

func (sa *SuggestedActions) DescribeActions() string {
	acts := make([]string, 0)
	for _, act := range sa.RepairActions {
		acts = append(acts, string(act))
	}
	return strings.Join(acts, ", ")
}

type EventType string

const (
	EventTypeUnknown EventType = "Unknown"

	// EventTypeInfo represents a general event that requires no action.
	// Info - Informative, no further action needed.
	EventTypeInfo EventType = "Info"

	// EventTypeWarning represents an event that may impact workloads.
	// Warning - Some issue happened but no further action needed, expecting automatic recovery.
	EventTypeWarning EventType = "Warning"

	// EventTypeCritical represents an event that is definitely impacting workloads
	// and requires immediate attention.
	// Critical - Some critical issue happened thus action required, not a hardware issue.
	EventTypeCritical EventType = "Critical"

	// EventTypeFatal represents a fatal event that impacts wide systems
	// and requires immediate attention and action.
	// Fatal - Fatal/hardware issue occurred thus immediate action required, may require reboot/hardware repair.
	EventTypeFatal EventType = "Fatal"
)

func EventTypeFromString(s string) EventType {
	switch s {
	case "Info":
		return EventTypeInfo
	case "Warning":
		return EventTypeWarning
	case "Critical":
		return EventTypeCritical
	case "Fatal":
		return EventTypeFatal
	default:
		return EventTypeUnknown
	}
}

type MachineInfo struct {
	// GPUdVersion represents the current version of GPUd
	GPUdVersion string `json:"gpudVersion,omitempty"`
	// GPUDriverVersion represents the current version of GPU driver installed
	GPUDriverVersion string `json:"gpuDriverVersion,omitempty"`
	// CUDAVersion represents the current version of cuda library.
	CUDAVersion string `json:"cudaVersion,omitempty"`
	// ContainerRuntime Version reported by the node through runtime remote API (e.g. containerd://1.4.2).
	ContainerRuntimeVersion string `json:"containerRuntimeVersion,omitempty"`
	// Kernel Version reported by the node from 'uname -r' (e.g. 3.16.0-0.bpo.4-amd64).
	KernelVersion string `json:"kernelVersion,omitempty"`
	// OS Image reported by the node from /etc/os-release (e.g. Debian GNU/Linux 7 (wheezy)).
	OSImage string `json:"osImage,omitempty"`
	// The Operating System reported by the node
	OperatingSystem string `json:"operatingSystem,omitempty"`
	// SystemUUID comes from https://github.com/google/cadvisor/blob/master/utils/sysfs/sysfs.go#L442
	SystemUUID string `json:"systemUUID,omitempty"`
	// MachineID is collected by GPUd. It comes from /etc/machine-id or /var/lib/dbus/machine-id
	MachineID string `json:"machineID,omitempty"`
	// BootID is collected by GPUd.
	BootID string `json:"bootID,omitempty"`
	// Hostname is the current host of machine
	Hostname string `json:"hostname,omitempty"`
	// Uptime represents when the machine up
	Uptime metav1.Time `json:"uptime,omitempty"`

	// CPUInfo is the CPU info of the machine.
	CPUInfo *MachineCPUInfo `json:"cpuInfo,omitempty"`
	// MemoryInfo is the memory info of the machine.
	MemoryInfo *MachineMemoryInfo `json:"memoryInfo,omitempty"`
	// GPUInfo is the GPU info of the machine.
	GPUInfo *MachineGPUInfo `json:"gpuInfo,omitempty"`
	// DiskInfo is the Disk info of the machine.
	DiskInfo *MachineDiskInfo `json:"diskInfo,omitempty"`
	// NICInfo is the network info of the machine.
	NICInfo *MachineNICInfo `json:"nicInfo,omitempty"`
}

func (i *MachineInfo) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.Append([]string{"GPUd Version", i.GPUdVersion})
	table.Append([]string{"Container Runtime Version", i.ContainerRuntimeVersion})
	table.Append([]string{"OS Image", i.OSImage})
	table.Append([]string{"Kernel Version", i.KernelVersion})

	if i.CPUInfo != nil {
		table.Append([]string{"CPU Type", i.CPUInfo.Type})
		table.Append([]string{"CPU Manufacturer", i.CPUInfo.Manufacturer})
		table.Append([]string{"CPU Architecture", i.CPUInfo.Architecture})
		table.Append([]string{"CPU Logical Cores", fmt.Sprintf("%d", i.CPUInfo.LogicalCores)})
	}
	if i.MemoryInfo != nil {
		table.Append([]string{"Memory Total", humanize.Bytes(i.MemoryInfo.TotalBytes)})
	}

	table.Append([]string{"CUDA Version", i.CUDAVersion})
	if i.GPUInfo != nil {
		table.Append([]string{"GPU Driver Version", i.GPUDriverVersion})
		table.Append([]string{"GPU Product", i.GPUInfo.Product})
		table.Append([]string{"GPU Manufacturer", i.GPUInfo.Manufacturer})
		table.Append([]string{"GPU Architecture", i.GPUInfo.Architecture})
		table.Append([]string{"GPU Memory", i.GPUInfo.Memory})
	}

	if i.NICInfo != nil {
		for idx, nic := range i.NICInfo.PrivateIPInterfaces {
			table.Append([]string{fmt.Sprintf("Private IP Interface %d", idx+1), fmt.Sprintf("%s (%s, %s)", nic.Interface, nic.MAC, nic.IP)})
		}
	}

	if i.DiskInfo != nil {
		table.Append([]string{"Container Root Disk", i.DiskInfo.ContainerRootDisk})
	}

	table.Render()
	fmt.Fprintf(wr, "\n")

	if i.DiskInfo != nil {
		i.DiskInfo.RenderTable(wr)
		fmt.Fprintf(wr, "\n")
	}

	if i.GPUInfo != nil {
		i.GPUInfo.RenderTable(wr)
		fmt.Fprintf(wr, "\n")
	}
}

type MachineCPUInfo struct {
	Type         string `json:"type,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	LogicalCores int64  `json:"logicalCores,omitempty"`
}

type MachineMemoryInfo struct {
	TotalBytes uint64 `json:"totalBytes"`
}

type MachineGPUInfo struct {
	// Product may be "NVIDIA-Graphics-Device" for NVIDIA GB200.
	Product string `json:"product,omitempty"`

	// Manufacturer is "NVIDIA" for NVIDIA GPUs (same as Brand).
	Manufacturer string `json:"manufacturer,omitempty"`

	// Architecture is "blackwell" for NVIDIA GB200.
	Architecture string `json:"architecture,omitempty"`

	Memory string `json:"memory,omitempty"`

	// GPUs is the GPU info of the machine.
	GPUs []MachineGPUInstance `json:"gpus,omitempty"`
}

type MachineGPUInstance struct {
	UUID    string `json:"uuid,omitempty"`
	SN      string `json:"sn,omitempty"`
	MinorID string `json:"minorID,omitempty"`
	BoardID uint32 `json:"boardID,omitempty"`
}

func (gi *MachineGPUInfo) RenderTable(wr io.Writer) {
	if len(gi.GPUs) > 0 {
		table := tablewriter.NewWriter(wr)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader([]string{"UUID", "SN", "MinorID"})

		for _, gpu := range gi.GPUs {
			table.Append([]string{
				gpu.UUID,
				gpu.SN,
				gpu.MinorID,
			})
		}

		table.Render()
	}
}

type MachineDiskInfo struct {
	BlockDevices []MachineDiskDevice `json:"blockDevices,omitempty"`
	// ContainerRootDisk is the disk device name that mounts the container root (such as "/var/lib/kubelet" mount point).
	ContainerRootDisk string `json:"containerRootDisk,omitempty"`
}

type MachineDiskDevice struct {
	Name       string   `json:"name,omitempty"`
	Type       string   `json:"type,omitempty"`
	Size       int64    `json:"size,omitempty"`
	Used       int64    `json:"used,omitempty"`
	Rota       bool     `json:"rota,omitempty"`
	Serial     string   `json:"serial,omitempty"`
	WWN        string   `json:"wwn,omitempty"`
	Vendor     string   `json:"vendor,omitempty"`
	Model      string   `json:"model,omitempty"`
	Rev        string   `json:"rev,omitempty"`
	MountPoint string   `json:"mountPoint,omitempty"`
	FSType     string   `json:"fsType,omitempty"`
	PartUUID   string   `json:"partUUID,omitempty"`
	Parents    []string `json:"parents,omitempty"`
	Children   []string `json:"children,omitempty"`
}

func (di *MachineDiskInfo) RenderTable(wr io.Writer) {
	if len(di.BlockDevices) > 0 {
		table := tablewriter.NewWriter(wr)
		table.SetAlignment(tablewriter.ALIGN_CENTER)
		table.SetHeader([]string{"Name", "Type", "FSType", "Used", "Size", "Mount Point", "Parents", "Children"})

		for _, blk := range di.BlockDevices {
			table.Append([]string{
				blk.Name,
				blk.Type,
				blk.FSType,
				humanize.Bytes(uint64(blk.Used)),
				humanize.Bytes(uint64(blk.Size)),
				blk.MountPoint,
				strings.Join(blk.Parents, "\n"),
				strings.Join(blk.Children, "\n"),
			})
		}

		table.Render()
	}
}

// MachineNetwork is the network info of the machine.
type MachineNetwork struct {
	// PublicIP is the public IP address of the machine.
	PublicIP string `json:"publicIP,omitempty"`
	// PrivateIP is the first private IP in IPv4 family,
	// detected from the local host.
	// May be overridden by the user with the private IP address.
	PrivateIP string `json:"privateIP,omitempty"`
}

// MachineNICInfo consists of the network info of the machine.
type MachineNICInfo struct {
	// PrivateIPInterfaces is the private network interface info of the machine.
	PrivateIPInterfaces []MachineNetworkInterface `json:"privateIPInterfaces,omitempty"`
}

// MachineNetworkInterface is the network interface info of the machine.
type MachineNetworkInterface struct {
	// Interface is the network interface name of the machine.
	Interface string `json:"interface,omitempty"`

	// MAC is the MAC address of the machine.
	MAC string `json:"mac,omitempty"`

	// IP is the string representation of the netip.Addr of the machine.
	IP string `json:"ip,omitempty"`

	// Addr is the netip.Addr of the machine.
	Addr netip.Addr `json:"-"`
}

// MachineLocation is the location info of the machine.
type MachineLocation struct {
	Region string `json:"region,omitempty"`
	Zone   string `json:"zone,omitempty"`
}
