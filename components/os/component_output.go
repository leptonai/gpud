package os

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/v4/host"
	procs "github.com/shirou/gopsutil/v4/process"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	os_id "github.com/leptonai/gpud/components/os/id"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/file"
	components_metrics "github.com/leptonai/gpud/pkg/gpud-metrics"
	pkg_host "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/query"
	"github.com/leptonai/gpud/pkg/reboot"
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
			Name:    os_id.Name,
			Healthy: true,
			Reason:  "os healthy",
		},
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

var (
	currentMachineMetadata    MachineMetadata
	currentSystemManufacturer string
)

func init() {
	// Linux-specific operations
	if runtime.GOOS != "linux" {
		return
	}

	// File descriptor limit check is Linux-specific
	if file.CheckFDLimitSupported() {
		limit, err := file.GetLimit()
		if limit > 0 && err == nil {
			// set to 20% of system limit
			DefaultZombieProcessCountThreshold = int(float64(limit) * 0.20)
		}
	}

	// Get boot ID (Linux-specific)
	var err error
	currentMachineMetadata.BootID, err = pkg_host.GetBootID()
	if err != nil {
		log.Logger.Warnw("failed to get boot id", "error", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	currentMachineMetadata.DmidecodeUUID, err = pkg_host.DmidecodeUUID(ctx)
	cancel()
	if err != nil {
		log.Logger.Debugw("failed to get dmidecode uuid", "error", err)
	}

	currentMachineMetadata.OSMachineID, err = pkg_host.GetOSMachineID()
	if err != nil {
		log.Logger.Warnw("failed to get os machine id", "error", err)
	}

	cctx, ccancel := context.WithTimeout(context.Background(), 20*time.Second)
	currentSystemManufacturer, err = pkg_host.SystemManufacturer(cctx)
	ccancel()
	if err != nil {
		log.Logger.Warnw("failed to get system manufacturer", "error", err)
	}
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config, eventBucket eventstore.Bucket) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			os_id.Name,
			cfg.Query,
			createGet(cfg, eventBucket),
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

var getSystemdDetectVirtFunc = pkg_host.SystemdDetectVirt

func createGet(cfg Config, eventBucket eventstore.Bucket) func(ctx context.Context) (_ any, e error) {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(os_id.Name)
			} else {
				components_metrics.SetGetSuccess(os_id.Name)
			}
		}()

		o := &Output{}

		cctx, ccancel := context.WithTimeout(ctx, 15*time.Second)
		virtEnv, err := getSystemdDetectVirtFunc(cctx)
		ccancel()
		if err != nil {
			return nil, fmt.Errorf("failed to get virtualization environment using 'systemd-detect-virt': %w", err)
		}
		o.VirtualizationEnvironment = virtEnv

		// for some reason, init failed
		if currentSystemManufacturer == "" && runtime.GOOS == "linux" {
			cctx, ccancel = context.WithTimeout(ctx, 20*time.Second)
			currentSystemManufacturer, err = pkg_host.SystemManufacturer(cctx)
			ccancel()
			if err != nil {
				log.Logger.Warnw("failed to get system manufacturer", "error", err)
			}
		}
		o.SystemManufacturer = currentSystemManufacturer

		o.MachineMetadata = currentMachineMetadata

		if err = createRebootEvent(ctx, eventBucket, reboot.LastReboot); err != nil {
			log.Logger.Warnw("failed to create reboot event", "error", err)
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

		cctx, ccancel = context.WithTimeout(ctx, 10*time.Second)
		uptime, err := host.UptimeWithContext(cctx)
		ccancel()
		if err != nil {
			return nil, err
		}

		cctx, ccancel = context.WithTimeout(ctx, 10*time.Second)
		boottime, err := host.BootTimeWithContext(cctx)
		ccancel()
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

		cctx, ccancel = context.WithTimeout(ctx, 10*time.Second)
		allProcs, err := process.CountProcessesByStatus(cctx)
		ccancel()
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

func createRebootEvent(ctx context.Context, eventBucket eventstore.Bucket, lastRebootTime func(ctx2 context.Context) (time.Time, error)) error {
	// get uptime
	bootTime, err := lastRebootTime(ctx)
	if err != nil {
		return err
	}
	// if now - event time > retention, then skip
	if time.Since(bootTime) >= DefaultRetentionPeriod {
		log.Logger.Debugw("skipping reboot event", "time_since", time.Since(bootTime), "retention", DefaultRetentionPeriod)
		return nil
	}
	// calculate event
	rebootEvent := components.Event{
		Time:    metav1.Time{Time: bootTime},
		Name:    "reboot",
		Type:    common.EventTypeWarning,
		Message: fmt.Sprintf("system reboot detected %v", bootTime),
	}
	// get db latest
	cctx, ccancel := context.WithTimeout(ctx, 10*time.Second)
	lastEvent, err := eventBucket.Latest(cctx)
	ccancel()
	if err != nil {
		return err
	}
	// if latest != "" and latest.timestamp == current event, skip
	if lastEvent != nil && lastEvent.Time.Time.Sub(bootTime).Abs() < time.Minute {
		return nil
	}
	// else insert event
	cctx, ccancel = context.WithTimeout(ctx, 10*time.Second)
	if err = eventBucket.Insert(cctx, rebootEvent); err != nil {
		ccancel()
		return err
	}
	ccancel()
	return nil
}
