// Package query implements "nvidia-smi --query" output helpers.
package query

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/leptonai/gpud/pkg/file"

	"sigs.k8s.io/yaml"
)

// Returns true if the local machine runs on Nvidia GPU
// by running "nvidia-smi".
func SMIExists() bool {
	p, err := file.LocateExecutable("nvidia-smi")
	if err != nil {
		return false
	}
	return p != ""
}

func RunSMI(ctx context.Context, args ...string) ([]byte, error) {
	p, err := file.LocateExecutable("nvidia-smi")
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi not found (%w)", err)
	}

	cmd := exec.CommandContext(ctx, p, args...)

	// in case of driver issue, the nvidia-smi is stuck in "state:D" -- uninterruptible sleep state
	// which may not handle the os kill signal from the context timeout/cancellation
	//
	// e.g.,
	//
	// [Sat Oct 12 18:35:32 2024] NVRM: Xid (PCI:0000:61:00): 45, pid=1496200, name=distprovers-gpu, Ch 0000000f
	// [Sat Oct 12 18:38:44 2024] INFO: task cache_mgr_event:620917 blocked for more than 120 seconds.
	// [Sat Oct 12 18:38:44 2024]       Tainted: P           OE     5.15.0-97-generic #107-Ubuntu
	// [Sat Oct 12 18:38:44 2024] "echo 0 > /proc/sys/kernel/hung_task_timeout_secs" disables this message.
	// [Sat Oct 12 18:38:44 2024] task:cache_mgr_event state:D stack:    0 pid:620917 ppid:620571 flags:0x00000002
	// [Sat Oct 12 18:38:44 2024] Call Trace:
	// [Sat Oct 12 18:38:44 2024]  <TASK>
	// [Sat Oct 12 18:38:44 2024]  __schedule+0x24e/0x590
	// [Sat Oct 12 18:38:44 2024]  schedule+0x69/0x110
	// [Sat Oct 12 18:38:44 2024]  rwsem_down_write_slowpath+0x230/0x3e0
	// [Sat Oct 12 18:38:44 2024]  ? load_new_mm_cr3+0x76/0xe0
	// [Sat Oct 12 18:38:44 2024]  down_write+0x47/0x60
	// [Sat Oct 12 18:38:44 2024]  os_acquire_rwlock_write+0x35/0x50 [nvidia]
	// [Sat Oct 12 18:38:44 2024]  _nv042330rm+0x10/0x40 [nvidia]
	// [Sat Oct 12 18:38:44 2024]  ? _nv043429rm+0x23c/0x290
	errc := make(chan error, 1)
	var output []byte
	go func() {
		var err error
		output, err = cmd.Output()
		errc <- err
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()

	case err := <-errc:
		if err != nil {
			return nil, fmt.Errorf("nvidia-smi command failed: %w", err)
		}
		return output, nil
	}
}

// Make sure to call this with a timeout, as a broken GPU may block the command.
// e.g.,
// nvAssertOkFailedNoLog: Assertion failed: Call timed out [NV_ERR_TIMEOUT] (0x00000065) returned from pRmApi->Control(pRmApi, RES_GET_CLIENT_HANDLE(pKernelChannel), RES_GET_HANDLE(pKernelChannel),
func GetSMIOutput(ctx context.Context) (*SMIOutput, error) {
	qb, err := RunSMI(ctx, "--query")
	if err != nil {
		return nil, err
	}
	o, err := ParseSMIQueryOutput(qb)
	if err != nil {
		return nil, err
	}

	sb, err := RunSMI(ctx)
	if err != nil {
		// e.g.,
		// Unable to determine the device handle for GPU0000:CB:00.0: Unknown Error
		if strings.Contains(err.Error(), "Unknown Error") {
			o.SummaryFailure = err
		} else {
			return nil, err
		}
	} else {
		if strings.Contains(string(sb), "Unknown Error") {
			o.SummaryFailure = errors.New(string(sb))
		} else {
			o.Summary = string(sb)
		}
	}

	return o, nil
}

// Represents the current nvidia status
// using "nvidia-smi --query", "nvidia-smi", etc..
// ref. "nvidia-smi --help-query-gpu"
type SMIOutput struct {
	Timestamp     string `json:"timestamp"`
	DriverVersion string `json:"driver_version"`
	CUDAVersion   string `json:"cuda_version"`
	AttachedGPUs  int    `json:"attached_gpus"`

	GPUs []NvidiaSMIGPU `json:"gpus,omitempty"`

	// Raw is the raw output of "nvidia-smi --query".
	// Useful for debugging.
	Raw string `json:"raw,omitempty"`

	// Summary is the "nvidia-smi" output without "--query" flag.
	// Useful for error detecting, in case the new nvidia-smi
	// version introduces breaking changes to its query output.
	Summary string `json:"summary,omitempty"`

	// Only set if "nvidia-smi" failed to run.
	SummaryFailure error `json:"summary_failure,omitempty"`
}

// ref. "nvidia-smi --help-query-gpu"
type rawSMIQueryOutput struct {
	Timestamp     string `json:"Timestamp"`
	DriverVersion string `json:"Driver Version"`
	CUDAVersion   string `json:"CUDA Version"`
	AttachedGPUs  int    `json:"Attached GPUs"`

	GPU0 *NvidiaSMIGPU `json:"GPU0,omitempty"`
	GPU1 *NvidiaSMIGPU `json:"GPU1,omitempty"`
	GPU2 *NvidiaSMIGPU `json:"GPU2,omitempty"`
	GPU3 *NvidiaSMIGPU `json:"GPU3,omitempty"`
	GPU4 *NvidiaSMIGPU `json:"GPU4,omitempty"`
	GPU5 *NvidiaSMIGPU `json:"GPU5,omitempty"`
	GPU6 *NvidiaSMIGPU `json:"GPU6,omitempty"`
	GPU7 *NvidiaSMIGPU `json:"GPU7,omitempty"`
}

type smiQueryOutputFallback struct {
	Timestamp     string `json:"Timestamp"`
	DriverVersion string `json:"Driver Version"`
	CUDAVersion   string `json:"CUDA Version"`
	AttachedGPUs  int    `json:"Attached GPUs"`
}

func (o *SMIOutput) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func (o *SMIOutput) YAML() ([]byte, error) {
	return yaml.Marshal(o)
}

// Decodes the "nvidia-smi --query" output.
// ref. https://developer.nvidia.com/system-management-interface
func ParseSMIQueryOutput(b []byte) (*SMIOutput, error) {
	splits := bytes.Split(b, []byte("\n"))
	processedLines := make([][]byte, 0, len(splits))

	// tracks the last line to its indent level
	lastIndent := 0
	gpuCursor := 0
	prevGPUID := ""

	for _, currentLine := range splits {
		if len(currentLine) == 0 {
			continue
		}
		if bytes.Contains(currentLine, []byte("===")) || bytes.Contains(currentLine, []byte("NVSMI LOG")) {
			continue
		}

		lastLine := []byte{}
		if len(processedLines) > 0 {
			lastLine = processedLines[len(processedLines)-1]
		}

		indentLevel := len(currentLine) - len(bytes.TrimSpace(currentLine))

		gpuIDLine := ""
		if prevGPUID != "" {
			gpuIDLine = strings.Repeat(" ", indentLevel) + "ID: " + prevGPUID
			prevGPUID = ""
		}

		lastKey := getKey(lastLine)
		switch {
		case bytes.HasPrefix(currentLine, []byte("GPU 00000")):
			// e.g.,
			//
			// GPU 00000000:53:00.0
			//
			// should be converted to
			//
			// GPU0

			prevGPUID = string(currentLine)
			currentLine = []byte(fmt.Sprintf("GPU%d:", gpuCursor))
			gpuCursor++

		case !bytes.HasSuffix(currentLine, []byte(":")) && !bytes.Contains(currentLine, []byte(":")):
			// e.g.,
			//
			//     Driver Model
			//          Current                           : N/A
			//
			// should be
			//
			//     Driver Model:
			//          Current                           : N/A

			currentLine = append(currentLine, ':')

		case bytes.HasSuffix(bytes.TrimSpace(currentLine), []byte("None")):
			// e.g.,
			//
			// Processes                             : None
			//
			// should be
			//
			// Processes                             : null
			currentLine = bytes.Replace(currentLine, []byte("None"), []byte("null"), 1)

		case bytes.Contains(currentLine, []byte("Remapped Rows")) && bytes.HasSuffix(bytes.TrimSpace(currentLine), []byte("N/A")):
			// e.g.,
			//
			// Remapped Rows                         : N/A
			//
			// should be
			//
			// Remapped Rows                         : null
			currentLine = bytes.Replace(currentLine, []byte("N/A"), []byte("null"), 1)

		case bytes.HasPrefix(lastKey, []byte("HW Slowdown")) ||
			bytes.HasPrefix(lastKey, []byte("HW Thermal Slowdown")) ||
			bytes.HasPrefix(lastKey, []byte("Process ID")) ||
			bytes.HasPrefix(lastKey, []byte("Process Type")) ||
			bytes.HasPrefix(lastKey, []byte("Process Name")):
			// e.g.,
			//
			// HW Slowdown                       : Not Active
			//     HW Thermal Slowdown           : Not Active
			//
			// should be
			//
			// HW Slowdown                   : Not Active
			// HW Thermal Slowdown           : Not Active

			// e.g.,
			//
			// Process ID                        : 1375347
			//     Type                          : C
			//     Name                          : /usr/bin/python
			//     Used GPU Memory               : 22372 MiB
			//
			// should be
			//
			// Process ID                        : 1375347
			// Process Type                      : C
			// Process Name                      : /usr/bin/python
			// Process Used GPU Memory           : 22372 MiB
			trimmed := bytes.TrimSpace(currentLine)
			currentLine = bytes.Repeat([]byte(" "), lastIndent)
			if bytes.HasPrefix(lastKey, []byte("Process ID")) ||
				bytes.HasPrefix(lastKey, []byte("Process Type")) ||
				bytes.HasPrefix(lastKey, []byte("Process Name")) {
				currentLine = append(currentLine, []byte("Process ")...)
			}

			currentLine = append(currentLine, trimmed...)

			// NOTE: for cases like 'Process Name: [celeryd: ...]'
			// it should be converted to 'Process Name: "[celeryd: ...]"'
			if bytes.Contains(currentLine, []byte("Process Name")) && bytes.Contains(currentLine, []byte(":")) {
				s := string(currentLine)
				splits := strings.Split(s, ":")

				key := splits[0]
				value := strings.TrimSpace(splits[1])
				currentLine = []byte(fmt.Sprintf("%s: %q", key, value))
			}
		}

		if gpuIDLine != "" {
			processedLines = append(processedLines, []byte(gpuIDLine))
		}

		processedLines = append(processedLines, currentLine)
		lastIndent = len(currentLine) - len(bytes.TrimSpace(currentLine))
	}

	processedOutput := bytes.Join(processedLines, []byte("\n"))

	raw := &rawSMIQueryOutput{}
	if err := yaml.Unmarshal(processedOutput, raw); err != nil {
		// in case nvidia-smi introduced some breaking changes
		// retry with a fallback implementation
		// and to retain debugging info such as driver version
		fallback := &smiQueryOutputFallback{}
		newOutput := bytes.Split(processedOutput, []byte("\nGPU"))[0]
		if rerr := yaml.Unmarshal(newOutput, fallback); rerr != nil {
			return nil, rerr
		}
		return &SMIOutput{
			Timestamp:     fallback.Timestamp,
			DriverVersion: fallback.DriverVersion,
			CUDAVersion:   fallback.CUDAVersion,
			AttachedGPUs:  fallback.AttachedGPUs,
		}, err
	}

	out := &SMIOutput{
		Timestamp:     raw.Timestamp,
		DriverVersion: raw.DriverVersion,
		CUDAVersion:   raw.CUDAVersion,
		AttachedGPUs:  raw.AttachedGPUs,
		Raw:           string(b),
	}
	gpuFields := []*NvidiaSMIGPU{raw.GPU0, raw.GPU1, raw.GPU2, raw.GPU3, raw.GPU4, raw.GPU5, raw.GPU6, raw.GPU7}
	for _, gpu := range gpuFields {
		if gpu != nil {
			out.GPUs = append(out.GPUs, *gpu)
		}
	}

	for i := range out.GPUs {
		id := out.GPUs[i].ID
		if out.GPUs[i].ECCErrors != nil {
			out.GPUs[i].ECCErrors.ID = id
		}
		if out.GPUs[i].Temperature != nil {
			out.GPUs[i].Temperature.ID = id
		}
		if out.GPUs[i].GPUPowerReadings != nil {
			out.GPUs[i].GPUPowerReadings.ID = id
		}
		if out.GPUs[i].FBMemoryUsage != nil {
			out.GPUs[i].FBMemoryUsage.ID = id
		}
	}

	return out, nil
}

// ref. https://forums.developer.nvidia.com/t/nvidia-smi-q-shows-several-unknown-error-gpu-ignored-by-pytorch/263881/2
func FindSummaryErr(s string) []string {
	errs := make([]string, 0)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if !strings.Contains(line, "ERR!") {
			continue
		}
		if i > 0 {
			errs = append(errs, lines[i-1]+"\n"+line)
			continue
		}
		errs = append(errs, line)
	}
	return errs
}

func getKey(line []byte) []byte {
	k := bytes.Split(line, []byte(":"))[0]
	return bytes.TrimSpace(k)
}

// Returns the detail GPU errors if any.
func (o *SMIOutput) FindGPUErrs() []string {
	rs := make([]string, 0)
	for _, g := range o.GPUs {
		errs := g.FindErrs()
		if len(errs) == 0 {
			continue
		}
		rs = append(rs, errs...)
	}

	if o.SummaryFailure != nil {
		rs = append(rs, o.SummaryFailure.Error())
	} else {
		if serrs := FindSummaryErr(o.Summary); len(serrs) > 0 {
			rs = append(rs, serrs...)
		}
	}

	if o.AttachedGPUs != len(o.GPUs) {
		rs = append(rs, fmt.Sprintf("AttachedGPUs %d != GPUs %d", o.AttachedGPUs, len(o.GPUs)))
	}

	if len(rs) == 0 {
		return nil
	}
	return rs
}

// Returns the detail HW Slowdown message if any of the GPU has "Active" HW Slowdown event.
func (o *SMIOutput) FindHWSlowdownErrs() []string {
	errs := make([]string, 0)
	for _, g := range o.GPUs {
		if g.ClockEventReasons == nil {
			continue
		}
		herrs := g.FindHWSlowdownErrs()
		if len(herrs) == 0 {
			continue
		}
		errs = append(errs, herrs...)
	}
	if len(errs) == 0 {
		return nil
	}
	return errs
}
