package server

import (
	"context"
	"embed"
	"html/template"
	stdos "os"
	"runtime"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/leptonai/gpud/components"
	nvidia_clock_speed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	nvidia_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	nvidia_hw_slowdown "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown"
	nvidia_memory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	nvidia_power "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	nvidia_temperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	nvidia_utilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	"github.com/leptonai/gpud/components/cpu"
	disk "github.com/leptonai/gpud/components/disk"
	"github.com/leptonai/gpud/components/fd"
	"github.com/leptonai/gpud/components/memory"
	"github.com/leptonai/gpud/pkg/config"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/version"
)

const rootTemplateName = "root.html"

//go:embed root.html
var rootTemplateFS embed.FS

var rootTmpl = template.Must(template.ParseFS(rootTemplateFS, rootTemplateName))

func createRootHandler(handlerDescs []componentHandlerDescription, webConfig config.Web) func(c *gin.Context) {
	pid := stdos.Getpid()

	cpuChart := false
	if c, err := components.GetComponent(cpu.Name); c != nil && err == nil {
		cpuChart = true
	}
	memoryChart := false
	if c, err := components.GetComponent(memory.Name); c != nil && err == nil {
		memoryChart = true
	}
	fdChart := false
	if c, err := components.GetComponent(fd.Name); c != nil && err == nil {
		fdChart = true
	}
	diskChart := false
	if c, err := components.GetComponent(disk.Name); c != nil && err == nil {
		diskChart = true
	}

	nvidiaGPUUtilChart := false
	nvidiaMemoryChart := false
	nvidiaTemperatureChart := false
	nvidiaPowerChart := false
	nvidiaClockSpeedChart := false
	nvidiaErrsChart := false

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	cancel()
	if err != nil {
		log.Logger.Fatalw("failed to check if nvidia is installed", "error", err)
	}

	if nvidiaInstalled {
		if c, err := components.GetComponent(nvidia_utilization.Name); c != nil && err == nil {
			nvidiaGPUUtilChart = true
		}
		if c, err := components.GetComponent(nvidia_memory.Name); c != nil && err == nil {
			nvidiaMemoryChart = true
		}
		if c, err := components.GetComponent(nvidia_temperature.Name); c != nil && err == nil {
			nvidiaTemperatureChart = true
		}
		if c, err := components.GetComponent(nvidia_power.Name); c != nil && err == nil {
			nvidiaPowerChart = true
		}
		if c, err := components.GetComponent(nvidia_clock_speed.Name); c != nil && err == nil {
			nvidiaClockSpeedChart = true
		}
		if c, err := components.GetComponent(nvidia_hw_slowdown.Name); c != nil && err == nil {
			nvidiaErrsChart = true
		}
		if c, err := components.GetComponent(nvidia_ecc.Name); c != nil && err == nil {
			nvidiaErrsChart = true
		}
	}

	components := []string{}
	if cpuChart {
		components = append(components, cpu.Name)
	}
	if memoryChart {
		components = append(components, memory.Name)
	}
	if fdChart {
		components = append(components, fd.Name)
	}
	if diskChart {
		components = append(components, disk.Name)
	}
	if nvidiaGPUUtilChart {
		components = append(components, nvidia_utilization.Name)
	}
	if nvidiaMemoryChart {
		components = append(components, nvidia_memory.Name)
	}
	if nvidiaTemperatureChart {
		components = append(components, nvidia_temperature.Name)
	}
	if nvidiaPowerChart {
		components = append(components, nvidia_power.Name)
	}
	if nvidiaClockSpeedChart {
		components = append(components, nvidia_clock_speed.Name)
	}
	if nvidiaErrsChart {
		components = append(components, nvidia_hw_slowdown.Name)
		components = append(components, nvidia_ecc.Name)
	}

	return func(c *gin.Context) {
		accelerator := "N/A"
		acceleratorDriver := "N/A"
		gpuAttached := "N/A"

		alloc, rss, err := getMemory(pid)
		if err != nil {
			log.Logger.Errorw("failed to get memory", "error", err)
			alloc = "N/A"
			rss = "N/A"
		}

		c.HTML(
			200,
			rootTemplateName,
			gin.H{
				"Version":           version.Version,
				"PID":               pid,
				"MemoryAlloc":       alloc,
				"MemoryRSS":         rss,
				"KernelArch":        pkghost.Arch(),
				"KernelVersion":     pkghost.KernelVersion(),
				"PlatformName":      pkghost.Platform(),
				"PlatformVersion":   pkghost.PlatformVersion(),
				"Accelerator":       accelerator,
				"AcceleratorDriver": acceleratorDriver,
				"GPUAttached":       gpuAttached,
				"Uptime":            "",
				"UptimeSeconds":     "",

				"Admin":                       webConfig.Admin,
				"Paths":                       handlerDescs,
				"MetricsSincePeriod":          webConfig.SincePeriod.Duration.String(),
				"RefreshPeriod":               webConfig.RefreshPeriod.Duration.String(),
				"RefreshPeriodInMilliseconds": webConfig.RefreshPeriod.Duration.Milliseconds(),

				"CPUChart":    cpuChart,
				"MemoryChart": memoryChart,
				"FDChart":     fdChart,
				"DiskChart":   diskChart,

				"Components":             strings.Join(components, ","),
				"NVIDIAClockSpeedChart":  nvidiaClockSpeedChart,
				"NVIDIAGPUUtilChart":     nvidiaGPUUtilChart,
				"NVIDIAMemoryChart":      nvidiaMemoryChart,
				"NVIDIATemperatureChart": nvidiaTemperatureChart,
				"NVIDIAPowerChart":       nvidiaPowerChart,
				"NVIDIAErrsChart":        nvidiaErrsChart,
			},
		)
	}
}

func getMemory(pid int) (string, string, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	proc, err := process.NewProcess(int32(pid))
	rss := "N/A"
	if err == nil {
		memInfo, err := proc.MemoryInfo()
		if err == nil {
			rss = humanize.Bytes(memInfo.RSS)
		}
	}

	return humanize.Bytes(m.Alloc), rss, nil
}
