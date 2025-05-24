package writer

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	pkgkmsg "github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

// CheckPermissions checks if the current user has the necessary permissions to run the gpud_kmsg_writer module.
func CheckPermissions() error {
	if runtime.GOOS != "linux" {
		return errors.New("only supported on Linux")
	}
	if os.Geteuid() != 0 {
		return errors.New("must be run as root")
	}
	return nil
}

// GPUdKmsgWriterModule defines the interface for the gpud_kmsg_writer module.
type GPUdKmsgWriterModule interface {
	// BuildInstall builds, installs, loads, and tests the gpud_kmsg_writer module.
	BuildInstall(context.Context) error
	// Uninstall uninstalls the gpud_kmsg_writer module.
	Uninstall(context.Context) error
	// InjectKernelMessage injects a message into the kernel log.
	InjectKernelMessage(context.Context, *KernelMessage) error
}

var _ GPUdKmsgWriterModule = &gpudKmsgWriterModule{}

type gpudKmsgWriterModule struct {
	ctx context.Context

	kmsgReadFunc  func(context.Context) ([]pkgkmsg.Message, error)
	processRunner process.Runner

	workDir         string
	cFile           string
	cFileContent    string
	makeFile        string
	makeFileContent string
	koFile          string
	devFile         string
	devName         string
	devMajorNum     string

	scriptBuildModule           string
	scriptReinstallLinuxHeaders string
	scriptInstallModule         string
	scriptUninstallModule       string

	createScriptLoadModuleFunc    func(devName string, devMajorNum string) string
	createScriptInjectMessageFunc func(devName string, msg *KernelMessage) string
}

var (
	//go:embed src/gpud_kmsg_writer.c
	gpudKmsgWriterC string

	//go:embed src/Makefile
	gpudKmsgWriterMakefile string
)

func NewGPUdKmsgWriterModule(
	ctx context.Context,
	workDir string,
) GPUdKmsgWriterModule {
	return &gpudKmsgWriterModule{
		ctx: ctx,

		kmsgReadFunc: pkgkmsg.ReadAll,

		// one shared runner for all the steps in this plugin
		// run them in sequence, one by one
		// this is to avoid running multiple commands in parallel
		processRunner: process.NewExclusiveRunner(),

		workDir:         workDir,
		cFile:           filepath.Join(workDir, "gpud_kmsg_writer.c"),
		cFileContent:    gpudKmsgWriterC,
		makeFile:        filepath.Join(workDir, "Makefile"),
		makeFileContent: gpudKmsgWriterMakefile,
		koFile:          filepath.Join(workDir, "gpud_kmsg_writer.ko"),
		devFile:         "/dev/gpud_kmsg_writer",
		devName:         "gpud_kmsg_writer",
		devMajorNum:     "",

		scriptBuildModule:           defaultScriptBuildModule,
		scriptReinstallLinuxHeaders: defaultScriptReinstallLinuxHeaders,
		scriptInstallModule:         "sudo insmod gpud_kmsg_writer.ko",
		scriptUninstallModule:       "sudo rmmod gpud_kmsg_writer",

		createScriptLoadModuleFunc:    createScriptLoadModule,
		createScriptInjectMessageFunc: createScriptInjectMessage,
	}
}

func (gi *gpudKmsgWriterModule) BuildInstall(ctx context.Context) error {
	if err := CheckPermissions(); err != nil {
		return err
	}

	log.Logger.Infow("building gpud_kmsg_writer module", "os", runtime.GOOS, "euid", os.Geteuid())

	if err := gi.cleanupFiles(); err != nil {
		return err
	}
	if err := gi.writeFiles(); err != nil {
		return err
	}

	if err := gi.buildModule(ctx); err != nil {
		return err
	}
	if err := gi.installModule(ctx); err != nil {
		return err
	}

	var err error
	gi.devMajorNum, err = gi.findMajorNum(ctx)
	if err != nil {
		return err
	}
	if err := gi.loadModule(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
	}

	if err := gi.injectKmsg(ctx, &KernelMessage{
		Priority: "KERN_DEBUG",
		Message:  "GPUd kmsg writer test message",
	}); err != nil {
		return err
	}

	log.Logger.Infow("successfully built and installed gpud_kmsg_writer module")
	return nil
}

func (gi *gpudKmsgWriterModule) Uninstall(ctx context.Context) error {
	if err := gi.uninstallModule(ctx); err != nil {
		return err
	}

	if err := gi.cleanupFiles(); err != nil {
		return err
	}

	return nil
}

func (gi *gpudKmsgWriterModule) cleanupFiles() error {
	if err := os.RemoveAll(gi.cFile); err != nil {
		return fmt.Errorf("failed to remove %q: %w", gi.cFile, err)
	}

	if err := os.RemoveAll(gi.koFile); err != nil {
		return fmt.Errorf("failed to remove %q: %w", gi.koFile, err)
	}

	if err := os.RemoveAll(gi.devFile); err != nil {
		return fmt.Errorf("failed to remove %q: %w", gi.devFile, err)
	}

	log.Logger.Infow("successfully cleaned up files", "cFile", gi.cFile, "koFile", gi.koFile, "devFile", gi.devFile)
	return nil
}

func (gi *gpudKmsgWriterModule) writeFiles() error {
	if err := os.WriteFile(gi.cFile, []byte(gi.cFileContent), 0644); err != nil {
		return fmt.Errorf("failed to write %q: %w", gi.cFile, err)
	}
	if err := os.WriteFile(gi.makeFile, []byte(gi.makeFileContent), 0644); err != nil {
		return fmt.Errorf("failed to write %q: %w", gi.makeFile, err)
	}

	log.Logger.Infow("successfully wrote files", "cFile", gi.cFile, "makeFile", gi.makeFile)
	return nil
}

const (
	defaultScriptBuildModule = `make -C /lib/modules/$(uname -r)/build M=$(pwd) modules`

	// some OS requires reinstalling linux headers
	// otherwise,
	// "ERROR: Kernel configuration is invalid."
	// "include/generated/autoconf.h or include/config/auto.conf are missing."
	defaultScriptReinstallLinuxHeaders = `
# reinstall Linux Headers
# ref. https://askubuntu.com/questions/890712/kernel-configuration-is-invalid-error-while-trying-to-install-paragon-ufsd-profe
sudo apt-get install -y --reinstall linux-headers-$(uname -r)
sudo apt-get install -y --reinstall -y build-essential dkms linux-generic
`
)

func (gi *gpudKmsgWriterModule) buildModule(ctx context.Context) error {
	log.Logger.Infow("building module", "workDir", gi.workDir)

	if err := os.Chdir(gi.workDir); err != nil {
		return fmt.Errorf("failed to change directory to %q: %w", gi.workDir, err)
	}

	execOut, exitCode, err := gi.processRunner.RunUntilCompletion(ctx, gi.scriptBuildModule)
	if err != nil {
		log.Logger.Errorw("failed to build module", "output", string(execOut), "exitCode", exitCode, "error", err)

		if strings.Contains(string(execOut), "Kernel configuration is invalid") {
			log.Logger.Infow("kernel configuration is invalid, reinstalling linux headers", "output", string(execOut), "exitCode", exitCode)
			execOut, exitCode, err = gi.processRunner.RunUntilCompletion(ctx, gi.scriptReinstallLinuxHeaders)
			if err != nil {
				log.Logger.Errorw("failed to reinstall linux headers", "output", string(execOut), "exitCode", exitCode, "error", err)
				return err
			}

			log.Logger.Infow("linux header reinstall script output", "output", string(execOut), "exitCode", exitCode)

			execOut, exitCode, err = gi.processRunner.RunUntilCompletion(ctx, gi.scriptBuildModule)
			if err != nil {
				log.Logger.Errorw("failed to build module after reinstalling linux headers", "output", string(execOut), "exitCode", exitCode, "error", err)
				return err
			}

			log.Logger.Infow("successfully built module after reinstalling linux headers", "output", string(execOut), "exitCode", exitCode)
		} else {
			return fmt.Errorf("failed to build module: %w", err)
		}
	}

	log.Logger.Infow("successfully built module", "output", string(execOut), "exitCode", exitCode)
	return nil
}

func (gi *gpudKmsgWriterModule) installModule(ctx context.Context) error {
	log.Logger.Infow("installing module", "workDir", gi.workDir)

	if err := os.Chdir(gi.workDir); err != nil {
		return fmt.Errorf("failed to change directory to %q: %w", gi.workDir, err)
	}

	execOut, exitCode, err := gi.processRunner.RunUntilCompletion(ctx, gi.scriptInstallModule)
	if err != nil {
		// e.g.,
		// "insmod: ERROR: could not insert module gpud_kmsg_writer.ko: File exists" with "exit status 1"
		if strings.Contains(string(execOut), "File exists") {
			log.Logger.Infow("module already installed, skipping", "output", string(execOut), "exitCode", exitCode)
			return nil
		}

		log.Logger.Errorw("failed to install module", "output", string(execOut), "exitCode", exitCode, "error", err)
		return err
	}

	log.Logger.Infow("successfully installed module", "output", string(execOut), "exitCode", exitCode)
	return nil
}

func (gi *gpudKmsgWriterModule) uninstallModule(ctx context.Context) error {
	log.Logger.Infow("uninstalling module", "workDir", gi.workDir)

	if err := os.Chdir(gi.workDir); err != nil {
		return fmt.Errorf("failed to change directory to %q: %w", gi.workDir, err)
	}

	execOut, exitCode, err := gi.processRunner.RunUntilCompletion(ctx, gi.scriptUninstallModule)
	if err != nil {
		log.Logger.Errorw("failed to uninstall module", "output", string(execOut), "exitCode", exitCode, "error", err)
		return err
	}

	log.Logger.Infow("successfully uninstalled module", "output", string(execOut), "exitCode", exitCode)
	return nil
}

func (gi *gpudKmsgWriterModule) findMajorNum(ctx context.Context) (string, error) {
	log.Logger.Infow("finding device major number")

	log.Logger.Infow("scanning kmsg for device major number")
	msgs, err := gi.kmsgReadFunc(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to read kmsg: %w", err)
	}
	// latest message (biggest timestamp) is the first one in the list
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Timestamp.After(msgs[j].Timestamp.Time)
	})

	majorNum := ""
	for _, msg := range msgs {
		if !strings.Contains(msg.Message, "module loaded with device major number") {
			continue
		}

		// e.g.,
		// grep "module loaded with device major number" | awk '{print $NF}'
		splits := strings.Fields(msg.Message)
		majorNum = splits[len(splits)-1]
		break
	}

	if majorNum == "" {
		return "", errors.New("failed to find device major number in kmsg")
	}

	return majorNum, nil
}

// Utility functions that don't need struct state
func createScriptLoadModule(devName string, devMajorNum string) string {
	return fmt.Sprintf("mknod /dev/%s c %s 0", devName, devMajorNum)
}

func (gi *gpudKmsgWriterModule) loadModule(ctx context.Context) error {
	log.Logger.Infow("loading module")

	scriptLoadModule := gi.createScriptLoadModuleFunc(gi.devName, gi.devMajorNum)
	execOut, exitCode, err := gi.processRunner.RunUntilCompletion(ctx, scriptLoadModule)
	if err != nil {
		log.Logger.Errorw("failed to load module", "output", string(execOut), "exitCode", exitCode, "error", err)
		return err
	}

	log.Logger.Infow("successfully loaded module", "output", string(execOut), "exitCode", exitCode)
	return nil
}

func createScriptInjectMessage(devName string, msg *KernelMessage) string {
	return fmt.Sprintf(`sudo sh -c "echo \"%s,%s\" > /dev/%s"`, msg.Priority, msg.Message, devName)
}

func (gi *gpudKmsgWriterModule) injectKmsg(ctx context.Context, msg *KernelMessage) error {
	if msg == nil {
		return nil
	}

	if err := msg.Validate(); err != nil {
		return fmt.Errorf("invalid message: %w", err)
	}
	log.Logger.Infow("injecting kernel message via module", "devName", gi.devName, "priority", msg.Priority, "msg", msg.Message)

	scriptInjectMsg := gi.createScriptInjectMessageFunc(gi.devName, msg)
	execOut, exitCode, err := gi.processRunner.RunUntilCompletion(ctx, scriptInjectMsg)
	if err != nil {
		log.Logger.Errorw("failed to test module", "output", string(execOut), "exitCode", exitCode, "error", err)
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(3 * time.Second):
	}

	log.Logger.Infow("scanning kmsg for checking injected message")
	msgs, err := gi.kmsgReadFunc(ctx)
	if err != nil {
		return fmt.Errorf("failed to read kmsg: %w", err)
	}
	// latest message (biggest timestamp) is the first one in the list
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].Timestamp.After(msgs[j].Timestamp.Time)
	})

	for _, m := range msgs {
		if strings.Contains(m.Message, msg.Message) {
			log.Logger.Infow("found message in kmsg", "timestamp", m.Timestamp.Time, "message", m.Message)
			return nil
		}
	}

	return errors.New("failed to find injected message in kmsg")
}

func (gi *gpudKmsgWriterModule) InjectKernelMessage(ctx context.Context, msg *KernelMessage) error {
	return gi.injectKmsg(ctx, msg)
}
