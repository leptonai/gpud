// Package diagnose provides a way to diagnose the system and components.
package diagnose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	"github.com/leptonai/gpud/pkg/process"
	pkd_systemd "github.com/leptonai/gpud/pkg/systemd"
)

type output struct {
	dir        string `json:"-"`
	rawDataDir string `json:"-"`

	CheckSummary []string        `json:"check_summary"`
	Results      []CommandResult `json:"results"`
}

type CommandResult struct {
	Command string `json:"command"`
	Error   string `json:"error,omitempty"`
}

func (o *output) YAML() ([]byte, error) {
	return yaml.Marshal(o)
}

func (o *output) SyncYAML(file string) error {
	if _, err := os.Stat(filepath.Dir(file)); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			return err
		}
	}
	data, err := o.YAML()
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0644)
}

func Run(ctx context.Context, opts ...OpOption) error {
	return run(ctx, getDir(), opts...)
}

func getDir() string {
	return fmt.Sprintf("gpud-diagnose-%s", time.Now().Format("2006-01-02_15-04-05"))
}

func run(ctx context.Context, dir string, opts ...OpOption) error {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return err
	}

	o := &output{
		dir:        dir,
		rawDataDir: filepath.Join(dir, "raw-data"),
	}
	for _, d := range []string{o.dir, o.rawDataDir} {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			if err = os.MkdirAll(d, 0755); err != nil {
				return err
			}
		}
	}

	fmt.Printf("%s diagnosing with directory %s\n", checkMark, dir)

	if err := o.checkUUID(ctx); err != nil {
		return err
	}
	if err := o.checkHostname(); err != nil {
		return err
	}

	if err := o.runCommand(ctx, "basic-info", "date"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "basic-info", "uptime"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "basic-info", "hwclock", "--verbose"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "basic-info", "uname", "-a"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "basic-info", "lscpu"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "basic-info", "cpupower", "frequency-info"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "basic-info", "runlevel"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "basic-info", "cat", "/etc/*release"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "basic-info", "ls", "/lib/modules/`uname -r`/kernel/drivers/video/*"); err != nil {
		return err
	}

	if err := o.runCommand(ctx, "systemlog", "cp", "/var/log/message*", filepath.Join(o.rawDataDir, "systemlog")+"/"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "systemlog", "cp", "/var/log/mcelog*", filepath.Join(o.rawDataDir, "systemlog")+"/"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "systemlog", "cp", "/var/log/syslog*", filepath.Join(o.rawDataDir, "systemlog")+"/"); err != nil {
		return err
	}

	// if the file size >32MB, truncate the latest 32 MB
	syslogFile := filepath.Join(o.rawDataDir, "systemlog") + "/syslog"
	if s, err := os.Stat(syslogFile); err == nil && s.Size() > 32*1024*1024 {
		if err := truncateKeepEnd(syslogFile, 32*1024*1024); err != nil {
			return err
		}
	}

	if err := o.runCommand(ctx, "systemlog", "cp", "/var/log/kern*", filepath.Join(o.rawDataDir, "systemlog")+"/"); err != nil {
		return err
	}
	if err := o.runCommand(ctx, "systemlog", "cp", "/var/log/dmesg*", filepath.Join(o.rawDataDir, "systemlog")+"/"); err != nil {
		return err
	}

	nvidiaInstalled, err := nvidia_query.GPUsInstalled(ctx)
	if nvidiaInstalled && err == nil {
		fmt.Printf("%s scanning kernel messages\n", inProgress)
		issueCnt, err := scanKmsg(ctx)
		if err != nil {
			o.Results = append(o.Results, CommandResult{Error: err.Error()})
		} else if issueCnt == 0 {
			o.CheckSummary = append(o.CheckSummary, "kernel messages scan passed")
		} else {
			o.CheckSummary = append(o.CheckSummary, fmt.Sprintf("kernel messages scan detected %d issues", issueCnt))
		}
	}

	if err := o.runCommand(ctx, "modprobe", "cp", "/etc/modprobe.d/*.*", filepath.Join(o.rawDataDir, "modprobe")+"/"); err != nil {
		return err
	}

	if commandExists("ipmitool") {
		if err := o.runCommand(ctx, "ipmitool", "ipmitool", "fru", "list"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "ipmitool", "ipmitool", "self", "list"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "ipmitool", "ipmitool", "mc", "info"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "ipmitool", "ipmitool", "sel", "elist"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "ipmitool", "ipmitool", "sensor", "list"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "ipmitool", "ipmitool", "sdr", "list"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "ipmitool", "ipmitool", "sel", "time", "get"); err != nil {
			return err
		}
	} else {
		o.Results = append(o.Results, CommandResult{
			Command: "ipmitool",
			Error:   "ipmitool is not installed",
		})
	}

	if commandExists("dmesg") {
		if err := o.runCommand(ctx, "dmesg", "dmesg"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "dmesg", "dmesg", "-T"); err != nil {
			return err
		}
	} else {
		o.Results = append(o.Results, CommandResult{
			Command: "dmesg",
			Error:   "dmesg is not installed",
		})
	}

	if commandExists("dmidecode") {
		if err := o.runCommand(ctx, "dmidecode", "dmidecode"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "dmidecode", "dmidecode", "-t", "slot"); err != nil {
			return err
		}
	} else {
		o.Results = append(o.Results, CommandResult{
			Command: "dmidecode",
			Error:   "dmidecode is not installed",
		})
	}

	if commandExists("lspci") {
		if err := o.runCommand(ctx, "lspci", "lspci"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "lspci", "lspci", "-v", "-d", "10de"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "lspci", "lspci", "-xxx", "-vvv", "-t"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "lspci", "lspci", "-xxx", "-vvv", "-b"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "lspci", "lspci", "-vvvvv"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "lspci", "lspci", "-nn"); err != nil {
			return err
		}
	} else {
		o.Results = append(o.Results, CommandResult{
			Command: "lspci",
			Error:   "lspci is not installed",
		})
	}

	if err := o.runCommand(ctx, "nvidia", "which", "nvidia-uninstall"); err != nil {
		return err
	}
	if pkd_systemd.SystemctlExists() {
		if err := o.runCommand(ctx, "systemd", "systemctl", "list-dependencies"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "systemd", "systemctl", "status", "gdm"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "systemd", "systemctl", "status", "nvidia-fabricmanager"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "systemd", "systemctl", "is-enabled", "nvidia-fabricmanager"); err != nil {
			return err
		}
	} else {
		o.Results = append(o.Results, CommandResult{
			Command: "systemctl",
			Error:   "systemctl is not installed",
		})
	}

	if exists, err := nvidia_query.GPUsInstalled(ctx); !exists || err != nil {
		o.Results = append(o.Results, CommandResult{
			Command: "nvidia-smi",
			Error:   "nvidia-smi is not installed",
		})
	} else {
		if _, err := os.Stat("nvidia-bug-report.sh"); err == nil {
			if err := o.runCommand(ctx, "nvidia", "nvidia-bug-report.sh", "--query", "--verbose"); err != nil {
				return err
			}
			if err := copyFile("nvidia-bug-report.log.gz", filepath.Join(dir, "nvidia-bug-report.log.gz")); err != nil {
				return err
			}
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "-pm", "1"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "-q"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "-a"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "topo", "-m"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "topo", "-mp"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "nvlink", "-s"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "nvlink", "-c"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "nvlink", "-e"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "nvlink", "-R"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "nvidia-smi", "nvlink", "-p"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "lsmod", "| grep -i nvidia"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "modinfo", "/lib/modules/`uname -r`/kernel/drivers/video/nvidia.ko"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "ps", "aux | grep -v grep | grep -i  nvidia"); err != nil {
			return err
		}
		if err := o.runCommand(ctx, "nvidia", "ps", "-ef | grep -v grep | grep -i  nvidia"); err != nil {
			return err
		}
	}

	if _, err := os.Stat("/var/log/fabricmanager.log"); err == nil {
		if err := copyFile("/var/log/fabricmanager.log", filepath.Join(o.rawDataDir, "nvidia", "fabricmanager.log")); err != nil {
			return err
		}
	} else {
		o.Results = append(o.Results, CommandResult{
			Command: "cat /var/log/fabricmanager.log",
			Error:   "/var/log/fabricmanager.log is not found",
		})
	}

	summaryFile := filepath.Join(dir, "summary.txt")
	if err := o.SyncYAML(summaryFile); err != nil {
		return err
	}
	if err := copyFile(summaryFile, "summary.txt"); err != nil {
		return err
	}

	if op.createArchive {
		// tar the directory into a single file
		tarFileName := dir + ".tar"
		if err := tarDirectory(dir, tarFileName); err != nil {
			return fmt.Errorf("failed to create tar archive: %w", err)
		}
		fmt.Printf("%s wrote %s (directory %s) -- see summary.txt\n", checkMark, tarFileName, dir)
		return nil
	}

	fmt.Printf("%s wrote to directory %s -- see summary.txt\n", checkMark, dir)
	return nil
}

func (o *output) checkUUID(ctx context.Context) error {
	if commandExists("dmidecode") {
		machineID, err := host.GetDmidecodeUUID(ctx)
		if err != nil {
			return err
		}

		if _, err := os.Stat(filepath.Join(o.rawDataDir, "basic-info")); os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Join(o.rawDataDir, "basic-info"), 0755); err != nil {
				return err
			}
		}
		uuidFile, err := os.Create(filepath.Join(o.rawDataDir, "basic-info", "uuid"))
		if err != nil {
			return err
		}
		defer uuidFile.Close()
		if _, err := uuidFile.WriteString(machineID); err != nil {
			return err
		}
	} else {
		o.Results = append(o.Results, CommandResult{
			Command: "dmidecode",
			Error:   "dmidecode is not found",
		})
	}
	return nil
}

func (o *output) checkHostname() error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(o.rawDataDir, "basic-info")); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Join(o.rawDataDir, "basic-info"), 0755); err != nil {
			return err
		}
	}
	f, err := os.Create(filepath.Join(o.rawDataDir, "basic-info", "hostname"))
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(hostname); err != nil {
		return err
	}

	return nil
}

func (o *output) runCommand(ctx context.Context, subDir string, args ...string) error {
	if !commandExists(args[0]) {
		o.Results = append(o.Results, CommandResult{
			Command: strings.Join(args, " "),
			Error:   fmt.Sprintf("%s is not installed", args[0]),
		})
		return nil
	}

	fileName := strings.Join(args, "-")
	fileName = strings.ReplaceAll(fileName, "*", "_matchall")
	fileName = strings.ReplaceAll(fileName, " ", "_")
	fileName = strings.ReplaceAll(fileName, "/", "_")
	fileName = strings.ReplaceAll(fileName, "`", "_")
	fileName = strings.ReplaceAll(fileName, "|", "_pipe")

	if _, err := os.Stat(filepath.Join(o.rawDataDir, subDir)); os.IsNotExist(err) {
		if err = os.MkdirAll(filepath.Join(o.rawDataDir, subDir), 0755); err != nil {
			return err
		}
	}
	f, err := os.Create(filepath.Join(o.rawDataDir, subDir, fileName))
	if err != nil {
		return err
	}
	defer f.Close()

	p, err := process.New(process.WithCommand(args...), process.WithRunAsBashScript(), process.WithOutputFile(f))
	if err != nil {
		return err
	}
	if serr := p.Start(ctx); serr != nil {
		return serr
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-p.Wait():
		if err != nil {
			o.Results = append(o.Results, CommandResult{
				Command: strings.Join(args, " "),
				Error:   err.Error(),
			})
		}
	}

	return nil
}
