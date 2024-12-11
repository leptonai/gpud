package server

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	stdos "os"
	"runtime"
	"strings"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_clockspeed "github.com/leptonai/gpud/components/accelerator/nvidia/clock-speed"
	nvidia_ecc "github.com/leptonai/gpud/components/accelerator/nvidia/ecc"
	nvidia_hw_slowdown_id "github.com/leptonai/gpud/components/accelerator/nvidia/hw-slowdown/id"
	nvidia_info "github.com/leptonai/gpud/components/accelerator/nvidia/info"
	nvidia_memory "github.com/leptonai/gpud/components/accelerator/nvidia/memory"
	nvidia_power "github.com/leptonai/gpud/components/accelerator/nvidia/power"
	nvidia_query "github.com/leptonai/gpud/components/accelerator/nvidia/query"
	nvidia_temperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	nvidia_utilization "github.com/leptonai/gpud/components/accelerator/nvidia/utilization"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/disk"
	fd_id "github.com/leptonai/gpud/components/fd/id"
	memory_id "github.com/leptonai/gpud/components/memory/id"
	"github.com/leptonai/gpud/components/os"
	"github.com/leptonai/gpud/config"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/version"

	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v4/process"
)

const rootTemplateName = "root.html"

//go:embed root.html
var rootTemplateFS embed.FS

var rootTmpl = template.Must(template.ParseFS(rootTemplateFS, rootTemplateName))

func createRootHandler(handlerDescs []componentHandlerDescription, webConfig config.Web) func(c *gin.Context) {
	pid := stdos.Getpid()

	osComponent, err := components.GetComponent(os.Name)
	if err != nil {
		panic(fmt.Sprintf("component %q required but not set", os.Name))
	}

	var osOutputProvider components.OutputProvider
	if outputProvider, ok := osComponent.(interface{ Unwrap() interface{} }); ok {
		if op, ok := outputProvider.Unwrap().(components.OutputProvider); ok {
			osOutputProvider = op
		} else {
			panic(fmt.Sprintf("component %q does not implement components.OutputProvider", os.Name))
		}
	}

	cpuChart := false
	if c, err := components.GetComponent(cpu.Name); c != nil && err == nil {
		cpuChart = true
	}
	memoryChart := false
	if c, err := components.GetComponent(memory_id.Name); c != nil && err == nil {
		memoryChart = true
	}
	fdChart := false
	if c, err := components.GetComponent(fd_id.Name); c != nil && err == nil {
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

	var nvidiaInfoOutputProvider components.OutputProvider
	if nvidiaInstalled {
		nvidiaInfoComponent, err := components.GetComponent(nvidia_info.Name)
		if err != nil {
			panic(fmt.Sprintf("component %q required but not set", nvidia_info.Name))
		}

		if uo, ok := nvidiaInfoComponent.(interface{ Unwrap() interface{} }); ok {
			if op, ok := uo.Unwrap().(components.OutputProvider); ok {
				nvidiaInfoOutputProvider = op
			} else {
				panic(fmt.Sprintf("component %q does not implement components.OutputProvider", nvidia_info.Name))
			}
		}

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
		if c, err := components.GetComponent(nvidia_clockspeed.Name); c != nil && err == nil {
			nvidiaClockSpeedChart = true
		}
		if c, err := components.GetComponent(nvidia_hw_slowdown_id.Name); c != nil && err == nil {
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
		components = append(components, memory_id.Name)
	}
	if fdChart {
		components = append(components, fd_id.Name)
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
		components = append(components, nvidia_clockspeed.Name)
	}
	if nvidiaErrsChart {
		components = append(components, nvidia_hw_slowdown_id.Name)
		components = append(components, nvidia_ecc.Name)
	}

	return func(c *gin.Context) {
		osOutputRaw, err := osOutputProvider.Output()
		if err != nil {
			panic(err)
		}
		osOutput, ok := osOutputRaw.(*os.Output)
		if !ok {
			panic(fmt.Sprintf("expected *os.Output, got %T", osOutputRaw))
		}

		accelerator := "N/A"
		acceleratorDriver := "N/A"
		gpuAttached := "N/A"
		if nvidiaInfoOutputProvider != nil {
			nvidiaInfoOutput, err := nvidiaInfoOutputProvider.Output()
			if err != nil {
				panic(err)
			}
			nvidiaInfo, ok := nvidiaInfoOutput.(*nvidia_info.Output)
			if !ok {
				panic(fmt.Sprintf("expected *nvidia_info.Output, got %T", nvidiaInfoOutput))
			}
			accelerator = fmt.Sprintf("%s (%s, %s)", nvidiaInfo.Product.Name, nvidiaInfo.Product.Brand, nvidiaInfo.Product.Architecture)
			acceleratorDriver = nvidiaInfo.Driver.Version
			gpuAttached = fmt.Sprintf("%d", nvidiaInfo.GPU.Attached)
		}

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
				"KernelArch":        osOutput.Kernel.Arch,
				"KernelVersion":     osOutput.Kernel.Version,
				"PlatformName":      osOutput.Platform.Name,
				"PlatformVersion":   osOutput.Platform.Version,
				"Accelerator":       accelerator,
				"AcceleratorDriver": acceleratorDriver,
				"GPUAttached":       gpuAttached,
				"Uptime":            osOutput.Uptimes.SecondsHumanized,
				"UptimeSeconds":     osOutput.Uptimes.Seconds,

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
