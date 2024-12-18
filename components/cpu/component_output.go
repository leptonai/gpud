// gopsutil is distributed under BSD license reproduced below.
//
// Copyright (c) 2014, WAKAYAMA Shirou
// All rights reserved.

package cpu

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	cpu_id "github.com/leptonai/gpud/components/cpu/id"
	"github.com/leptonai/gpud/components/cpu/metrics"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/query"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
)

type Output struct {
	Info  Info  `json:"info"`
	Cores Cores `json:"cores"`
	Usage Usage `json:"usage"`
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

type Info struct {
	Arch      string `json:"arch"`
	CPU       string `json:"cpu"`
	Family    string `json:"family"`
	Model     string `json:"model"`
	ModelName string `json:"model_name"`
}

type Cores struct {
	Physical int `json:"physical"`
	Logical  int `json:"logical"`
}

type Usage struct {
	// Used CPU in percentage.
	UsedPercent string `json:"used_percent"`

	// Load average for the last 1-minute, with the scale of 1.00.
	LoadAvg1Min string `json:"load_avg_1min"`
	// Load average for the last 5-minutes, with the scale of 1.00.
	LoadAvg5Min string `json:"load_avg_5min"`
	// Load average for the last 15-minutes, with the scale of 1.00.
	LoadAvg15Min string `json:"load_avg_15min"`
}

func (u Usage) GetUsedPercent() (float64, error) {
	return strconv.ParseFloat(u.UsedPercent, 64)
}

func (u Usage) GetLoadAvg1Min() (float64, error) {
	return strconv.ParseFloat(u.LoadAvg1Min, 64)
}

func (u Usage) GetLoadAvg5Min() (float64, error) {
	return strconv.ParseFloat(u.LoadAvg5Min, 64)
}

func (u Usage) GetLoadAvg15Min() (float64, error) {
	return strconv.ParseFloat(u.LoadAvg15Min, 64)
}

const (
	StateNameInfo         = "info"
	StateKeyInfoCPU       = "cpu"
	StateKeyInfoArch      = "arch"
	StateKeyInfoFamily    = "family"
	StateKeyInfoModel     = "model"
	StateKeyInfoModelName = "model_name"

	StateNameCores        = "cores"
	StateKeyCoresPhysical = "physical"
	StateKeyCoresLogical  = "logical"

	StateNameUsage            = "usage"
	StateKeyUsageUsedPercent  = "used_percent"
	StateKeyUsageLoadAvg1Min  = "load_avg_1min"
	StateKeyUsageLoadAvg5Min  = "load_avg_5min"
	StateKeyUsageLoadAvg15Min = "load_avg_15min"
)

func ParseStateInfo(m map[string]string) (Info, error) {
	i := Info{}
	i.Arch = m[StateKeyInfoArch]
	i.CPU = m[StateKeyInfoCPU]
	i.Family = m[StateKeyInfoFamily]
	i.Model = m[StateKeyInfoModel]
	i.ModelName = m[StateKeyInfoModelName]
	return i, nil
}

func ParseStateKeyCores(m map[string]string) (Cores, error) {
	c := Cores{}

	var err error
	c.Physical, err = strconv.Atoi(m[StateKeyCoresPhysical])
	if err != nil {
		return Cores{}, err
	}
	c.Logical, err = strconv.Atoi(m[StateKeyCoresLogical])
	if err != nil {
		return Cores{}, err
	}

	return c, nil
}

func ParseStateUsage(m map[string]string) (Usage, error) {
	u := Usage{}
	u.UsedPercent = m[StateKeyUsageUsedPercent]
	u.LoadAvg1Min = m[StateKeyUsageLoadAvg1Min]
	u.LoadAvg5Min = m[StateKeyUsageLoadAvg5Min]
	u.LoadAvg15Min = m[StateKeyUsageLoadAvg15Min]
	return u, nil
}

func ParseStatesToOutput(states ...components.State) (*Output, error) {
	o := &Output{}
	for _, state := range states {
		switch state.Name {
		case StateNameInfo:
			info, err := ParseStateInfo(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Info = info

		case StateNameCores:
			cores, err := ParseStateKeyCores(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Cores = cores

		case StateNameUsage:
			usage, err := ParseStateUsage(state.ExtraInfo)
			if err != nil {
				return nil, err
			}
			o.Usage = usage

		default:
			return nil, fmt.Errorf("unknown state name: %s", state.Name)
		}
	}
	return o, nil
}

func (o *Output) States() ([]components.State, error) {
	return []components.State{
		{
			Name:    StateNameInfo,
			Healthy: true,
			Reason: fmt.Sprintf("arch: %s, cpu: %s, family: %s, model: %s, model_name: %s",
				o.Info.Arch, o.Info.CPU, o.Info.Family, o.Info.Model, o.Info.ModelName),
			ExtraInfo: map[string]string{
				StateKeyInfoArch:      o.Info.Arch,
				StateKeyInfoCPU:       o.Info.CPU,
				StateKeyInfoFamily:    o.Info.Family,
				StateKeyInfoModel:     o.Info.Model,
				StateKeyInfoModelName: o.Info.ModelName,
			},
		},
		{
			Name:    StateNameCores,
			Healthy: true,
			Reason:  fmt.Sprintf("physical: %d cores, logical: %d cores", o.Cores.Physical, o.Cores.Logical),
			ExtraInfo: map[string]string{
				StateKeyCoresPhysical: fmt.Sprintf("%d", o.Cores.Physical),
				StateKeyCoresLogical:  fmt.Sprintf("%d", o.Cores.Logical),
			},
		},
		{
			Name:    StateNameUsage,
			Healthy: true,
			Reason:  fmt.Sprintf("used_percent: %s, load_avg_1min: %s, load_avg_5min: %s, load_avg_15min: %s", o.Usage.UsedPercent, o.Usage.LoadAvg1Min, o.Usage.LoadAvg5Min, o.Usage.LoadAvg15Min),
			ExtraInfo: map[string]string{
				StateKeyUsageUsedPercent:  o.Usage.UsedPercent,
				StateKeyUsageLoadAvg1Min:  o.Usage.LoadAvg1Min,
				StateKeyUsageLoadAvg5Min:  o.Usage.LoadAvg5Min,
				StateKeyUsageLoadAvg15Min: o.Usage.LoadAvg15Min,
			},
		},
	}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			cpu_id.Name,
			cfg.Query,
			Get,
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

const perCPU = false

var (
	prevMu sync.RWMutex
	prev   *cpu.TimesStat
)

func setPrevTimeStat(t cpu.TimesStat) {
	prevMu.Lock()
	defer prevMu.Unlock()
	prev = &t
}

func getPrevTimeStat() *cpu.TimesStat {
	prevMu.RLock()
	defer prevMu.RUnlock()
	return prev
}

func Get(ctx context.Context) (_ any, e error) {
	defer func() {
		if e != nil {
			components_metrics.SetGetFailed(cpu_id.Name)
		} else {
			components_metrics.SetGetSuccess(cpu_id.Name)
		}
	}()

	o := &Output{}

	arch, err := host.KernelArch()
	if err != nil {
		return nil, err
	}

	infos, err := cpu.InfoWithContext(ctx)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("no cpu info found")
	}
	o.Info = Info{
		Arch:      arch,
		CPU:       infos[0].ModelName,
		Family:    infos[0].Family,
		Model:     infos[0].Model,
		ModelName: infos[0].ModelName,
	}

	physicalCores, err := cpu.CountsWithContext(ctx, false)
	if err != nil {
		return nil, err
	}
	logicalCores, err := cpu.CountsWithContext(ctx, true)
	if err != nil {
		return nil, err
	}
	o.Cores = Cores{
		Physical: physicalCores,
		Logical:  logicalCores,
	}

	cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
	timeStats, err := cpu.TimesWithContext(cctx, perCPU)
	ccancel()
	if err != nil {
		return nil, err
	}
	if len(timeStats) != 1 {
		return nil, fmt.Errorf("expected 1 cpu time stat, got %d", len(timeStats))
	}

	now := time.Now().UTC()
	nowUnix := float64(now.Unix())
	metrics.SetLastUpdateUnixSeconds(nowUnix)

	prev := getPrevTimeStat()
	cur := timeStats[0]
	if prev == nil {
		cctx, ccancel := context.WithTimeout(ctx, 5*time.Second)
		usages, err := cpu.PercentWithContext(cctx, 0, perCPU)
		ccancel()
		if err != nil {
			return nil, err
		}
		if len(usages) != 1 {
			return nil, fmt.Errorf("expected 1 cpu usage, got %d", len(usages))
		}

		if err := metrics.SetUsedPercent(ctx, usages[0], now); err != nil {
			return nil, err
		}
		o.Usage = Usage{
			UsedPercent: fmt.Sprintf("%.2f", usages[0]),
		}
	} else {
		pct := calculateBusy(*prev, cur)
		if err := metrics.SetUsedPercent(ctx, pct, now); err != nil {
			return nil, err
		}
		o.Usage = Usage{
			UsedPercent: fmt.Sprintf("%.2f", pct),
		}
	}
	setPrevTimeStat(cur)

	cctx, ccancel = context.WithTimeout(ctx, 5*time.Second)
	loadAvg, err := load.AvgWithContext(cctx)
	ccancel()
	if err != nil {
		return nil, err
	}

	if err := metrics.SetLoadAverage(ctx, time.Minute, loadAvg.Load1, now); err != nil {
		return nil, err
	}
	o.Usage.LoadAvg1Min = fmt.Sprintf("%.2f", loadAvg.Load1)

	if err := metrics.SetLoadAverage(ctx, 5*time.Minute, loadAvg.Load5, now); err != nil {
		return nil, err
	}
	o.Usage.LoadAvg5Min = fmt.Sprintf("%.2f", loadAvg.Load5)

	if err := metrics.SetLoadAverage(ctx, 15*time.Minute, loadAvg.Load15, now); err != nil {
		return nil, err
	}
	o.Usage.LoadAvg15Min = fmt.Sprintf("%.2f", loadAvg.Load15)

	return o, nil
}

// copied from https://pkg.go.dev/github.com/shirou/gopsutil/v4/cpu#PercentWithContext
func calculateBusy(t1, t2 cpu.TimesStat) float64 {
	t1All, t1Busy := getAllBusy(t1)
	t2All, t2Busy := getAllBusy(t2)

	if t2Busy <= t1Busy {
		return 0
	}
	if t2All <= t1All {
		return 100
	}
	return math.Min(100, math.Max(0, (t2Busy-t1Busy)/(t2All-t1All)*100))
}

// copied from https://pkg.go.dev/github.com/shirou/gopsutil/v4/cpu#PercentWithContext
func getAllBusy(t cpu.TimesStat) (float64, float64) {
	//nolint:staticcheck // Allowing use of deprecated fields
	tot := t.Total()
	if runtime.GOOS == "linux" {
		tot -= t.Guest     // Linux 2.6.24+
		tot -= t.GuestNice // Linux 3.2.0+
	}

	busy := tot - t.Idle - t.Iowait

	return tot, busy
}
