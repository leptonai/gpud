package os

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/components/state"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/file"
	pkg_host "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/process"

	"github.com/shirou/gopsutil/v4/host"
	procs "github.com/shirou/gopsutil/v4/process"
)

type Output struct {
	VirtualizationEnvironment   pkg_host.VirtualizationEnvironment `json:"virtualization_environment"`
	SystemManufacturer          string                             `json:"system_manufacturer"`
	MachineMetadata             MachineMetadata                    `json:"machine_metadata"`
	MachineRebooted             bool                               `json:"machine_rebooted"`
	Host                        Host                               `json:"host"`
	Kernel                      Kernel                             `json:"kernel"`
	Platform                    Platform                           `json:"platform"`
	Uptimes                     Uptimes                            `json:"uptimes"`
	ProcessCountZombieProcesses int                                `json:"process_count_zombie_processes"`
}

type Host struct {
	ID string `json:"id"`
}

type Kernel struct {
	Arch    string `json:"arch"`
	Version string `json:"version"`
}

type Platform struct {
	Name    string `json:"name"`
	Family  string `json:"family"`
	Version string `json:"version"`
}

type Uptimes struct {
	Seconds          uint64 `json:"seconds"`
	SecondsHumanized string `json:"seconds_humanized"`

	BootTimeUnixSeconds uint64 `json:"boot_time_unix_seconds"`
	BootTimeHumanized   string `json:"boot_time_humanized"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNameVirtualizationEnvironment         = "virtualization_environment"
	StateKeyVirtualizationEnvironmentType      = "type"
	StateKeyVirtualizationEnvironmentVM        = "vm"
	StateKeyVirtualizationEnvironmentContainer = "container"
	StateKeyVirtualizationEnvironmentIsKVM     = "is_kvm"

	StateNameSystemManufacturer = "system_manufacturer"
	StateKeySystemManufacturer  = "system_manufacturer"

	StateNameMachineMetadata             = "machine_metadata"
	StateKeyMachineMetadataBootID        = "boot_id"
	StateKeyMachineMetadataDmidecodeUUID = "dmidecode_uuid"
	StateKeyMachineMetadataOSMachineID   = "os_machine_id"

	StateNameHost  = "host"
	StateKeyHostID = "id"

	StateNameKernel       = "kernel"
	StateKeyKernelArch    = "arch"
	StateKeyKernelVersion = "version"

	StateNamePlatform       = "platform"
	StateKeyPlatformName    = "name"
	StateKeyPlatformFamily  = "family"
	StateKeyPlatformVersion = "version"

	StateNameUptimes                   = "uptimes"
	StateKeyUptimesSeconds             = "uptime_seconds"
	StateKeyUptimesHumanized           = "uptime_humanized"
	StateKeyUptimesBootTimeUnixSeconds = "boot_time_unix_seconds"
	StateKeyUptimesBootTimeHumanized   = "boot_time_humanized"

	StateNameProcessCountsByStatus      = "process_counts_by_status"
	StateKeyProcessCountZombieProcesses = "process_count_zombie_processes"
)

func ParseStateVirtualizationEnvironment(m map[string]string) (pkg_host.VirtualizationEnvironment, error) {
	virtEnv := pkg_host.VirtualizationEnvironment{}
	virtEnv.Type = m[StateKeyVirtualizationEnvironmentType]
	virtEnv.VM = m[StateKeyVirtualizationEnvironmentVM]
	virtEnv.Container = m[StateKeyVirtualizationEnvironmentContainer]

	var err error
	virtEnv.IsKVM, err = strconv.ParseBool(m[StateKeyVirtualizationEnvironmentIsKVM])
	return virtEnv, err
}

func ParseStateSystemManufacturer(m map[string]string) (string, error) {
	return m[StateKeySystemManufacturer], nil
}

func ParseStateMachineMetadata(m map[string]string) (MachineMetadata, error) {
	mm := MachineMetadata{}
	mm.BootID = m[StateKeyMachineMetadataBootID]
	mm.DmidecodeUUID = m[StateKeyMachineMetadataDmidecodeUUID]
	mm.OSMachineID = m[StateKeyMachineMetadataOSMachineID]
	return mm, nil
}

func ParseStateHost(m map[string]string) (Host, error) {
	h := Host{}
	h.ID = m[StateKeyHostID]
	return h, nil
}

func ParseStateKernel(m map[string]string) (Kernel, error) {
	k := Kernel{}
	k.Arch = m[StateKeyKernelArch]
	k.Version = m[StateKeyKernelVersion]
	return k, nil
}

func ParseStatePlatform(m map[string]string) (Platform, error) {
	p := Platform{}
	p.Name = m[StateKeyPlatformName]
	p.Family = m[StateKeyPlatformFamily]
	p.Version = m[StateKeyPlatformVersion]
	return p, nil
}

func ParseStateUptimes(m map[string]string) (Uptimes, error) {
	u := Uptimes{}

	var err error
	u.Seconds, err = strconv.ParseUint(m[StateKeyUptimesSeconds], 10, 64)
	if err != nil {
		return Uptimes{}, err
	}
	u.SecondsHumanized = m[StateKeyUptimesHumanized]

	u.BootTimeUnixSeconds, err = strconv.ParseUint(m[StateKeyUptimesBootTimeUnixSeconds], 10, 64)
	if err != nil {
		return Uptimes{}, err
	}
	u.BootTimeHumanized = m[StateKeyUptimesBootTimeHumanized]

	return u, nil
}

func ParseStateProcessCountZombieProcesses(m map[string]string) (int, error) {
	s, ok := m[StateKeyProcessCountZombieProcesses]
	if ok {
		count, err := strconv.Atoi(s)
		if err != nil {
			return 0, err
		}
		return count, nil
	}
	return 0, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	o := &Output{}
	for _, state := range states {
		switch state.Name {
		case Name:
			// noop

		case StateNameVirtualizationEnvironment:
			virtEnv, err := ParseStateVirtualizationEnvironment(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.VirtualizationEnvironment = virtEnv

		case StateNameSystemManufacturer:
			manufacturer, err := ParseStateSystemManufacturer(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.SystemManufacturer = manufacturer

		case StateNameMachineMetadata:
			mm, err := ParseStateMachineMetadata(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.MachineMetadata = mm

		case StateNameHost:
			host, err := ParseStateHost(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Host = host

		case StateNameKernel:
			kernel, err := ParseStateKernel(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Kernel = kernel

		case StateNamePlatform:
			platform, err := ParseStatePlatform(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Platform = platform

		case StateNameUptimes:
			uptimes, err := ParseStateUptimes(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Uptimes = uptimes

		case StateNameProcessCountsByStatus:
			var err error
			o.ProcessCountZombieProcesses, err = ParseStateProcessCountZombieProcesses(state.ExtraInfo)
			if err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return o, nil
}

func (o *Output) States() ([]components.State, error) {
	virtReason := fmt.Sprintf("type: %s, vm: %s, container: %s", o.VirtualizationEnvironment.Type, o.VirtualizationEnvironment.VM, o.VirtualizationEnvironment.Container)
	if o.VirtualizationEnvironment.Type == "" {
		virtReason = "not detected"
	}

	manufacturerReason := fmt.Sprintf("manufacturer: %s", o.SystemManufacturer)
	if o.SystemManufacturer == "" {
		manufacturerReason = "not detected"
	}

	states := []components.State{
		{
			Name:    StateNameVirtualizationEnvironment,
			Healthy: true,
			Reason:  virtReason,
			ExtraInfo: map[string]string{
				StateKeyVirtualizationEnvironmentType:      o.VirtualizationEnvironment.Type,
				StateKeyVirtualizationEnvironmentVM:        o.VirtualizationEnvironment.VM,
				StateKeyVirtualizationEnvironmentContainer: o.VirtualizationEnvironment.Container,
				StateKeyVirtualizationEnvironmentIsKVM:     fmt.Sprintf("%v", o.VirtualizationEnvironment.IsKVM),
			},
		},
		{
			Name:    StateNameSystemManufacturer,
			Healthy: true,
			Reason:  manufacturerReason,
			ExtraInfo: map[string]string{
				StateKeySystemManufacturer: o.SystemManufacturer,
			},
		},
		{
			Name:    StateNameMachineMetadata,
			Healthy: true,
			Reason:  fmt.Sprintf("boot id: %s, dmidecode uuid: %s, os machine id: %s", currentMachineMetadata.BootID, currentMachineMetadata.DmidecodeUUID, currentMachineMetadata.OSMachineID),
			ExtraInfo: map[string]string{
				StateKeyMachineMetadataBootID:        currentMachineMetadata.BootID,
				StateKeyMachineMetadataDmidecodeUUID: currentMachineMetadata.DmidecodeUUID,
				StateKeyMachineMetadataOSMachineID:   currentMachineMetadata.OSMachineID,
			},
		},
		{
			Name:    StateNameHost,
			Healthy: true,
			Reason:  fmt.Sprintf("host id: %s", o.Host.ID),
			ExtraInfo: map[string]string{
				StateKeyHostID: o.Host.ID,
			},
		},
		{
			Name:    StateNameKernel,
			Healthy: true,
			Reason:  fmt.Sprintf("arch: %s, version: %s", o.Kernel.Arch, o.Kernel.Version),
			ExtraInfo: map[string]string{
				StateKeyKernelArch:    o.Kernel.Arch,
				StateKeyKernelVersion: o.Kernel.Version,
			},
		},
		{
			Name:    StateNamePlatform,
			Healthy: true,
			Reason:  fmt.Sprintf("name: %s, family: %s, version: %s", o.Platform.Name, o.Platform.Family, o.Platform.Version),
			ExtraInfo: map[string]string{
				StateKeyPlatformName:    o.Platform.Name,
				StateKeyPlatformFamily:  o.Platform.Family,
				StateKeyPlatformVersion: o.Platform.Version,
			},
		},
		{
			Name:    StateNameUptimes,
			Healthy: true,
			Reason:  fmt.Sprintf("uptime: %s, boot time: %s", o.Uptimes.SecondsHumanized, o.Uptimes.BootTimeHumanized),
			ExtraInfo: map[string]string{
				StateKeyUptimesSeconds:             fmt.Sprintf("%d", o.Uptimes.Seconds),
				StateKeyUptimesHumanized:           o.Uptimes.SecondsHumanized,
				StateKeyUptimesBootTimeUnixSeconds: fmt.Sprintf("%d", o.Uptimes.BootTimeUnixSeconds),
				StateKeyUptimesBootTimeHumanized:   o.Uptimes.BootTimeHumanized,
			},
		},
	}

	stateProcCounts := components.State{
		Name:    StateNameProcessCountsByStatus,
		Healthy: true,
		ExtraInfo: map[string]string{
			StateKeyProcessCountZombieProcesses: fmt.Sprintf("%d", o.ProcessCountZombieProcesses),
		},
	}
	if o.ProcessCountZombieProcesses >= DefaultZombieProcessCountThreshold {
		stateProcCounts.Healthy = false
		stateProcCounts.Reason = fmt.Sprintf("too many zombie processes: %d (threshold: %d)", o.ProcessCountZombieProcesses, DefaultZombieProcessCountThreshold)
	} else {
		stateProcCounts.Reason = fmt.Sprintf("zombie processes: %d (threshold: %d)", o.ProcessCountZombieProcesses, DefaultZombieProcessCountThreshold)
	}

	states = append(states, stateProcCounts)
	return states, nil
}

var DefaultZombieProcessCountThreshold = 1000

type MachineMetadata struct {
	BootID        string `json:"boot_id"`
	DmidecodeUUID string `json:"dmidecode_uuid"`
	OSMachineID   string `json:"os_machine_id"`
}

var currentMachineMetadata MachineMetadata

func init() {
	// e.g., "/proc/sys/fs/file-max" exists on linux
	if file.CheckFDLimitSupported() {
		limit, err := file.GetLimit()
		if limit > 0 && err == nil {
			// set to 20% of system limit
			DefaultZombieProcessCountThreshold = int(float64(limit) * 0.20)
		}
	}

	var err error
	currentMachineMetadata.BootID, err = pkg_host.GetBootID()
	if err != nil {
		log.Logger.Warnw("failed to get boot id", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	currentMachineMetadata.DmidecodeUUID, err = pkg_host.DmidecodeUUID(ctx)
	cancel()
	if err != nil {
		log.Logger.Warnw("failed to get dmidecode uuid", "error", err)
	}

	currentMachineMetadata.OSMachineID, err = pkg_host.GetOSMachineID()
	if err != nil {
		log.Logger.Warnw("failed to get os machine id", "error", err)
	}
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(Name, cfg.Query, CreateGet(cfg))
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) func(ctx context.Context) (_ any, e error) {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(Name)
			} else {
				components_metrics.SetGetSuccess(Name)
			}
		}()

		o := &Output{}

		cctx, ccancel := context.WithTimeout(ctx, 10*time.Second)
		virtEnv, err := pkg_host.SystemdDetectVirt(cctx)
		ccancel()
		if err != nil {
			return nil, err
		}
		o.VirtualizationEnvironment = virtEnv

		cctx, ccancel = context.WithTimeout(ctx, 10*time.Second)
		manufacturer, err := pkg_host.SystemManufacturer(cctx)
		ccancel()
		if err != nil {
			return nil, err
		}
		o.SystemManufacturer = manufacturer

		o.MachineMetadata = currentMachineMetadata

		cctx, ccancel = context.WithTimeout(ctx, 10*time.Second)
		lastBootID, err := state.GetLastBootID(cctx, cfg.Query.State.DB)
		ccancel()
		if err != nil {
			return nil, err
		}

		// boot id must be non-empty to correctly evaluate if the machine rebooted
		o.MachineRebooted = lastBootID != "" && currentMachineMetadata.BootID != "" && lastBootID != currentMachineMetadata.BootID

		// 1. empty (new GPUd version without reboot, boot id never persisted)
		// 2. different ID (rebooted and boot id changed, e.g., new GPUd version)
		if currentMachineMetadata.BootID != "" && lastBootID != currentMachineMetadata.BootID {
			if err := state.InsertBootID(cctx, cfg.Query.State.DB, currentMachineMetadata.BootID, time.Now().UTC()); err != nil {
				return nil, err
			}
			// next os poll will set the rebooted to false
		}

		hostID, err := host.HostID()
		if err != nil {
			return nil, err
		}
		o.Host = Host{ID: hostID}

		arch, err := host.KernelArch()
		if err != nil {
			return nil, err
		}
		kernelVer, err := host.KernelVersion()
		if err != nil {
			return nil, err
		}
		o.Kernel = Kernel{Arch: arch, Version: kernelVer}

		platform, family, version, err := host.PlatformInformation()
		if err != nil {
			return nil, err
		}
		o.Platform = Platform{Name: platform, Family: family, Version: version}

		uptime, err := host.UptimeWithContext(ctx)
		if err != nil {
			return nil, err
		}
		boottime, err := host.BootTimeWithContext(ctx)
		if err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		o.Uptimes = Uptimes{
			Seconds:             uptime,
			SecondsHumanized:    humanize.RelTime(now.Add(time.Duration(-int64(uptime))*time.Second), now, "ago", "from now"),
			BootTimeUnixSeconds: boottime,
			BootTimeHumanized:   humanize.RelTime(time.Unix(int64(boottime), 0), now, "ago", "from now"),
		}

		allProcs, err := process.CountProcessesByStatus(ctx)
		if err != nil {
			return nil, err
		}

		for status, procsWithStatus := range allProcs {
			if status == procs.Zombie {
				o.ProcessCountZombieProcesses = len(procsWithStatus)
				break
			}
		}

		return o, nil
	}
}
