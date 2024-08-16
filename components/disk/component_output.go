package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/disk/metrics"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"

	"github.com/dustin/go-humanize"
	"github.com/shirou/gopsutil/v4/disk"
)

type Output struct {
	ExtPartitions []Partition `json:"ext_partitions"`
	Usages        []Usage     `json:"usages"`
}

type Partition struct {
	Device     string `json:"device"`
	MountPoint string `json:"mount_point"`
	Fstype     string `json:"fstype"`
}

func getPartitions() ([]Partition, error) {
	partitions, err := disk.Partitions(true)
	if err != nil {
		return nil, err
	}
	ps := make([]Partition, 0, len(partitions))
	for _, p := range partitions {
		ps = append(ps, Partition{
			Device:     p.Device,
			MountPoint: p.Mountpoint,
			Fstype:     p.Fstype,
		})
	}
	return ps, nil
}

type Usage struct {
	MountPoint string `json:"path"`
	Fstype     string `json:"fstype"`

	TotalBytes     uint64 `json:"total_bytes"`
	TotalHumanized string `json:"total_humanized"`

	FreeBytes     uint64 `json:"free_bytes"`
	FreeHumanized string `json:"free_humanized"`

	UsedBytes     uint64 `json:"used_bytes"`
	UsedHumanized string `json:"used_humanized"`

	UsedPercent      string  `json:"used_percent"`
	UsedPercentFloat float64 `json:"-"`

	InodesTotal       uint64 `json:"inodes_total"`
	InodesUsed        uint64 `json:"inodes_used"`
	InodesFree        uint64 `json:"inodes_free"`
	InodesUsedPercent string `json:"inodes_used_percent"`

	InodesUsedPercentFloat float64 `json:"-"`
}

func getUsage(path string) (Usage, error) {
	usage, err := disk.Usage(path)
	if err != nil {
		return Usage{}, err
	}
	return Usage{
		MountPoint:             usage.Path,
		Fstype:                 usage.Fstype,
		TotalBytes:             usage.Total,
		TotalHumanized:         humanize.Bytes(usage.Total),
		FreeBytes:              usage.Free,
		FreeHumanized:          humanize.Bytes(usage.Free),
		UsedBytes:              usage.Used,
		UsedHumanized:          humanize.Bytes(usage.Used),
		UsedPercent:            fmt.Sprintf("%.2f", usage.UsedPercent),
		UsedPercentFloat:       usage.UsedPercent,
		InodesTotal:            usage.InodesTotal,
		InodesUsed:             usage.InodesUsed,
		InodesFree:             usage.InodesFree,
		InodesUsedPercent:      fmt.Sprintf("%.2f", usage.InodesUsedPercent),
		InodesUsedPercentFloat: usage.InodesUsedPercent,
	}, nil
}

func (u Usage) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(u.UsedPercent, 64)
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
	StateNameDiskExtPartition = "disk_ext_partition"

	StateKeyDiskPartitionDevice     = "device"
	StateKeyDiskPartitionMountPoint = "mount_point"
	StateKeyDiskPartitionFstype     = "fstype"

	StateNameDiskUsage = "disk_usage"

	StateKeyDiskUsageMountPoint        = "mount_point"
	StateKeyDiskUsageFstype            = "fstype"
	StateKeyDiskUsageTotalBytes        = "total_bytes"
	StateKeyDiskUsageTotalHumanized    = "total_humanized"
	StateKeyDiskUsageFreeBytes         = "free_bytes"
	StateKeyDiskUsageFreeHumanized     = "free_humanized"
	StateKeyDiskUsageUsedBytes         = "used_bytes"
	StateKeyDiskUsageUsedHumanized     = "used_humanized"
	StateKeyDiskUsageUsedPercent       = "used_percent"
	StateKeyDiskUsageInodesTotal       = "inodes_total"
	StateKeyDiskUsageInodesUsed        = "inodes_used"
	StateKeyDiskUsageInodesFree        = "inodes_free"
	StateKeyDiskUsageInodesUsedPercent = "inodes_used_percent"
)

func (p Partition) Map() map[string]string {
	return map[string]string{
		StateKeyDiskPartitionDevice:     p.Device,
		StateKeyDiskPartitionMountPoint: p.MountPoint,
		StateKeyDiskPartitionFstype:     p.Fstype,
	}
}

func (u Usage) Map() map[string]string {
	return map[string]string{
		StateKeyDiskUsageMountPoint:        u.MountPoint,
		StateKeyDiskUsageFstype:            u.Fstype,
		StateKeyDiskUsageTotalBytes:        fmt.Sprintf("%d", u.TotalBytes),
		StateKeyDiskUsageTotalHumanized:    u.TotalHumanized,
		StateKeyDiskUsageFreeBytes:         fmt.Sprintf("%d", u.FreeBytes),
		StateKeyDiskUsageFreeHumanized:     u.FreeHumanized,
		StateKeyDiskUsageUsedBytes:         fmt.Sprintf("%d", u.UsedBytes),
		StateKeyDiskUsageUsedHumanized:     u.UsedHumanized,
		StateKeyDiskUsageUsedPercent:       u.UsedPercent,
		StateKeyDiskUsageInodesTotal:       fmt.Sprintf("%d", u.InodesTotal),
		StateKeyDiskUsageInodesUsed:        fmt.Sprintf("%d", u.InodesUsed),
		StateKeyDiskUsageInodesFree:        fmt.Sprintf("%d", u.InodesFree),
		StateKeyDiskUsageInodesUsedPercent: u.InodesUsedPercent,
	}
}

func ParseStateDiskPartition(m map[string]string) (Partition, error) {
	p := Partition{}

	p.Device = m[StateKeyDiskPartitionDevice]
	p.MountPoint = m[StateKeyDiskPartitionMountPoint]
	p.Fstype = m[StateKeyDiskPartitionFstype]

	return p, nil
}

func ParseStateDiskUsage(m map[string]string) (Usage, error) {
	u := Usage{}

	u.MountPoint = m[StateKeyDiskUsageMountPoint]
	u.Fstype = m[StateKeyDiskUsageFstype]

	var err error
	u.TotalBytes, err = strconv.ParseUint(m[StateKeyDiskUsageTotalBytes], 10, 64)
	if err != nil {
		return Usage{}, err
	}
	u.TotalHumanized = m[StateKeyDiskUsageTotalHumanized]

	u.FreeBytes, err = strconv.ParseUint(m[StateKeyDiskUsageFreeBytes], 10, 64)
	if err != nil {
		return Usage{}, err
	}
	u.FreeHumanized = m[StateKeyDiskUsageFreeHumanized]

	u.UsedBytes, err = strconv.ParseUint(m[StateKeyDiskUsageUsedBytes], 10, 64)
	if err != nil {
		return Usage{}, err
	}
	u.UsedHumanized = m[StateKeyDiskUsageUsedHumanized]

	u.UsedPercent = m[StateKeyDiskUsageUsedPercent]

	u.InodesTotal, err = strconv.ParseUint(m[StateKeyDiskUsageInodesTotal], 10, 64)
	if err != nil {
		return Usage{}, err
	}
	u.InodesUsed, err = strconv.ParseUint(m[StateKeyDiskUsageInodesUsed], 10, 64)
	if err != nil {
		return Usage{}, err
	}
	u.InodesFree, err = strconv.ParseUint(m[StateKeyDiskUsageInodesFree], 10, 64)
	if err != nil {
		return Usage{}, err
	}
	u.InodesUsedPercent = m[StateKeyDiskUsageInodesUsedPercent]

	return u, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	o := &Output{}
	for _, state := range states {
		switch state.Name {
		case StateNameDiskExtPartition:
			partition, err := ParseStateDiskPartition(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.ExtPartitions = append(o.ExtPartitions, partition)

		case StateNameDiskUsage:
			usage, err := ParseStateDiskUsage(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Usages = append(o.Usages, usage)

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return o, nil
}

func (o *Output) States() ([]components.State, error) {
	cs := make([]components.State, 0, len(o.ExtPartitions)+len(o.Usages))
	for _, p := range o.ExtPartitions {
		cs = append(cs, components.State{
			Name:      StateNameDiskExtPartition,
			Healthy:   true,
			Reason:    fmt.Sprintf("device: %s, mount_point: %s, fstype: %s", p.Device, p.MountPoint, p.Fstype),
			ExtraInfo: p.Map(),
		})
	}
	for _, usage := range o.Usages {
		cs = append(cs, components.State{
			Name:      StateNameDiskUsage,
			Healthy:   true,
			Reason:    fmt.Sprintf("mount_point: %s, fstype: %s, used percent: %s (using %s out of %s)", usage.MountPoint, usage.Fstype, usage.UsedPercent, usage.UsedHumanized, usage.TotalHumanized),
			ExtraInfo: usage.Map(),
		})
	}
	return cs, nil
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

func CreateGet(cfg Config) query.GetFunc {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(Name)
			} else {
				components_metrics.SetGetSuccess(Name)
			}
		}()

		o := &Output{}

		partitions, err := getPartitions()
		if err != nil {
			return nil, err
		}
		for _, p := range partitions {
			if strings.Contains(p.Fstype, "ext") {
				o.ExtPartitions = append(o.ExtPartitions, p)
			}
		}

		now := time.Now().UTC()
		nowUTC := float64(now.Unix())
		metrics.SetLastUpdateUnixSeconds(nowUTC)

		for _, path := range cfg.MountPoints {
			usage, err := getUsage(path)
			if err != nil {
				return nil, err
			}
			o.Usages = append(o.Usages, usage)

			if err := metrics.SetTotalBytes(ctx, usage.MountPoint, float64(usage.TotalBytes), now); err != nil {
				return nil, err
			}
			metrics.SetFreeBytes(usage.MountPoint, float64(usage.FreeBytes))
			if err := metrics.SetUsedBytes(ctx, usage.MountPoint, float64(usage.UsedBytes), now); err != nil {
				return nil, err
			}
			if err := metrics.SetUsedBytesPercent(ctx, usage.MountPoint, usage.UsedPercentFloat, now); err != nil {
				return nil, err
			}
			metrics.SetUsedInodesPercent(usage.MountPoint, usage.InodesUsedPercentFloat)
		}

		return o, nil
	}
}
