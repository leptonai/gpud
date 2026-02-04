package host

import (
	"context"
	"errors"
	"io"
	stdos "os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	gocpu "github.com/shirou/gopsutil/v4/cpu"
	gohost "github.com/shirou/gopsutil/v4/host"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/process"
)

type hostState struct {
	hostID              string
	arch                string
	vendorID            string
	cpuModelName        string
	cpuModel            string
	cpuFamily           string
	cpuLogicalCores     int
	kernelVersion       string
	platform            string
	platformFamily      string
	platformVersion     string
	bootTimeUnixSeconds uint64
	bootID              string
	machineID           string
	dmidecodeUUID       string
	virtEnv             VirtualizationEnvironment
	systemManufacturer  string
	osMachineID         string
	osName              string
	systemUUID          string
}

func snapshotHostState() hostState {
	return hostState{
		hostID:              currentHostID,
		arch:                currentArch,
		vendorID:            currentVendorID,
		cpuModelName:        currentCPUModelName,
		cpuModel:            currentCPUModel,
		cpuFamily:           currentCPUFamily,
		cpuLogicalCores:     currentCPULogicalCores,
		kernelVersion:       currentKernelVersion,
		platform:            currentPlatform,
		platformFamily:      currentPlatformFamily,
		platformVersion:     currentPlatformVersion,
		bootTimeUnixSeconds: currentBootTimeUnixSeconds,
		bootID:              currentBootID,
		machineID:           currentMachineID,
		dmidecodeUUID:       currentDmidecodeUUID,
		virtEnv:             currentVirtEnv,
		systemManufacturer:  currentSystemManufacturer,
		osMachineID:         currentOSMachineID,
		osName:              currentOSName,
		systemUUID:          currentSystemUUID,
	}
}

func restoreHostState(saved hostState) {
	currentHostID = saved.hostID
	currentArch = saved.arch
	currentVendorID = saved.vendorID
	currentCPUModelName = saved.cpuModelName
	currentCPUModel = saved.cpuModel
	currentCPUFamily = saved.cpuFamily
	currentCPULogicalCores = saved.cpuLogicalCores
	currentKernelVersion = saved.kernelVersion
	currentPlatform = saved.platform
	currentPlatformFamily = saved.platformFamily
	currentPlatformVersion = saved.platformVersion
	currentBootTimeUnixSeconds = saved.bootTimeUnixSeconds
	currentBootID = saved.bootID
	currentMachineID = saved.machineID
	currentDmidecodeUUID = saved.dmidecodeUUID
	currentVirtEnv = saved.virtEnv
	currentSystemManufacturer = saved.systemManufacturer
	currentOSMachineID = saved.osMachineID
	currentOSName = saved.osName
	currentSystemUUID = saved.systemUUID
}

type fakeProcess struct {
	startErr error
	closeErr error
	waitErr  error
	started  bool
	closed   bool
	waitCh   chan error
}

func newFakeProcess(startErr, closeErr, waitErr error) *fakeProcess {
	ch := make(chan error, 1)
	ch <- waitErr
	return &fakeProcess{
		startErr: startErr,
		closeErr: closeErr,
		waitErr:  waitErr,
		waitCh:   ch,
	}
}

func (p *fakeProcess) Start(_ context.Context) error {
	p.started = true
	return p.startErr
}

func (p *fakeProcess) Started() bool {
	return p.started
}

func (p *fakeProcess) StartAndWaitForCombinedOutput(_ context.Context) ([]byte, error) {
	return nil, nil
}

func (p *fakeProcess) Close(_ context.Context) error {
	p.closed = true
	return p.closeErr
}

func (p *fakeProcess) Closed() bool {
	return p.closed
}

func (p *fakeProcess) Wait() <-chan error {
	return p.waitCh
}

func (p *fakeProcess) PID() int32 {
	return 0
}

func (p *fakeProcess) ExitCode() int32 {
	return 0
}

func (p *fakeProcess) StdoutReader() io.Reader {
	return strings.NewReader("")
}

func (p *fakeProcess) StderrReader() io.Reader {
	return strings.NewReader("")
}

func TestLoadInfo_ErrorPaths(t *testing.T) {
	saved := snapshotHostState()
	t.Cleanup(func() {
		restoreHostState(saved)
	})

	mockey.PatchConvey("loadInfo handles errors", t, func() {
		mockey.Mock(gohost.HostID).To(func() (string, error) {
			return "", errors.New("host id failed")
		}).Build()
		mockey.Mock(gohost.KernelArch).To(func() (string, error) {
			return "", errors.New("kernel arch failed")
		}).Build()
		mockey.Mock(gocpu.InfoWithContext).To(func(_ context.Context) ([]gocpu.InfoStat, error) {
			return []gocpu.InfoStat{{VendorID: "", ModelName: "undefined cpu", Model: "", Family: ""}}, nil
		}).Build()
		mockey.Mock(gocpu.CountsWithContext).To(func(_ context.Context, _ bool) (int, error) {
			return 0, errors.New("cpu count failed")
		}).Build()
		mockey.Mock(gohost.KernelVersion).To(func() (string, error) {
			return "", errors.New("kernel version failed")
		}).Build()
		mockey.Mock(gohost.PlatformInformation).To(func() (string, string, string, error) {
			return "", "", "", errors.New("platform info failed")
		}).Build()
		mockey.Mock(gohost.BootTimeWithContext).To(func(_ context.Context) (uint64, error) {
			return 0, errors.New("boot time failed")
		}).Build()

		mockey.Mock(GetBootID).To(func() (string, error) {
			return "", errors.New("boot id failed")
		}).Build()
		mockey.Mock(GetMachineID).To(func(_ context.Context) (string, error) {
			return "", errors.New("machine id failed")
		}).Build()
		mockey.Mock(GetDmidecodeUUID).To(func(_ context.Context) (string, error) {
			return "", errors.New("dmidecode uuid failed")
		}).Build()
		mockey.Mock(GetSystemdDetectVirt).To(func(_ context.Context) (VirtualizationEnvironment, error) {
			return VirtualizationEnvironment{}, errors.New("detect virt failed")
		}).Build()
		mockey.Mock(GetSystemManufacturer).To(func(_ context.Context) (string, error) {
			return "", errors.New("system manufacturer failed")
		}).Build()
		mockey.Mock(GetOSMachineID).To(func() (string, error) {
			return "", errors.New("os machine id failed")
		}).Build()
		mockey.Mock(GetOSName).To(func() (string, error) {
			return "", errors.New("os name failed")
		}).Build()
		mockey.Mock(GetSystemUUID).To(func() (string, error) {
			return "", errors.New("system uuid failed")
		}).Build()

		loadInfo()
	})
}

func TestReboot_DelayContextCancel(t *testing.T) {
	mockey.PatchConvey("reboot delay path with canceled context", t, func() {
		mockey.Mock(stdos.Geteuid).To(func() int {
			return 0
		}).Build()

		called := false
		mockey.Mock(runReboot).To(func(_ context.Context, _ string) error {
			called = true
			return nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := Reboot(ctx, WithDelaySeconds(1))
		require.NoError(t, err)

		time.Sleep(20 * time.Millisecond)
		require.False(t, called)
	})
}

func TestStop_DelayContextCancel(t *testing.T) {
	mockey.PatchConvey("stop delay path with canceled context", t, func() {
		mockey.Mock(stdos.Geteuid).To(func() int {
			return 0
		}).Build()

		called := false
		mockey.Mock(runStop).To(func(_ context.Context, _ string) error {
			called = true
			return nil
		}).Build()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := Stop(ctx, WithDelaySeconds(1))
		require.NoError(t, err)

		time.Sleep(20 * time.Millisecond)
		require.False(t, called)
	})
}

func TestRunReboot_ProcessNewError(t *testing.T) {
	mockey.PatchConvey("runReboot returns error when process.New fails", t, func() {
		mockey.Mock(process.New).To(func(_ ...process.OpOption) (process.Process, error) {
			return nil, errors.New("process new failed")
		}).Build()

		err := runReboot(context.Background(), "sudo reboot")
		require.Error(t, err)
	})
}

func TestRunStop_ProcessNewError(t *testing.T) {
	mockey.PatchConvey("runStop returns error when process.New fails", t, func() {
		mockey.Mock(process.New).To(func(_ ...process.OpOption) (process.Process, error) {
			return nil, errors.New("process new failed")
		}).Build()

		err := runStop(context.Background(), "sudo systemctl stop gpud")
		require.Error(t, err)
	})
}

func TestGetSystemdDetectVirt_ReadError(t *testing.T) {
	mockey.PatchConvey("GetSystemdDetectVirt returns error on read failure", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(_ string) (string, error) {
			return "/bin/systemd-detect-virt", nil
		}).Build()
		mockey.Mock(process.New).To(func(_ ...process.OpOption) (process.Process, error) {
			return newFakeProcess(nil, nil, nil), nil
		}).Build()
		mockey.Mock(process.Read).To(func(_ context.Context, _ process.Process, _ ...process.ReadOpOption) error {
			return errors.New("read failed")
		}).Build()

		_, err := GetSystemdDetectVirt(context.Background())
		require.Error(t, err)
	})
}

func TestGetSystemManufacturer_ReadError(t *testing.T) {
	mockey.PatchConvey("GetSystemManufacturer returns error on read failure", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(_ string) (string, error) {
			return "/bin/dmidecode", nil
		}).Build()
		mockey.Mock(process.New).To(func(_ ...process.OpOption) (process.Process, error) {
			return newFakeProcess(nil, nil, nil), nil
		}).Build()
		mockey.Mock(process.Read).To(func(_ context.Context, _ process.Process, _ ...process.ReadOpOption) error {
			return errors.New("read failed")
		}).Build()

		_, err := GetSystemManufacturer(context.Background())
		require.Error(t, err)
	})
}

func TestGetDmidecodeUUID_ReadError(t *testing.T) {
	mockey.PatchConvey("GetDmidecodeUUID returns error on read failure", t, func() {
		mockey.Mock(file.LocateExecutable).To(func(_ string) (string, error) {
			return "/bin/dmidecode", nil
		}).Build()
		mockey.Mock(process.New).To(func(_ ...process.OpOption) (process.Process, error) {
			return newFakeProcess(nil, nil, nil), nil
		}).Build()
		mockey.Mock(process.Read).To(func(_ context.Context, _ process.Process, _ ...process.ReadOpOption) error {
			return errors.New("read failed")
		}).Build()

		_, err := GetDmidecodeUUID(context.Background())
		require.Error(t, err)
	})
}

func TestGetOSName_FileMissing(t *testing.T) {
	missingFile := filepath.Join(t.TempDir(), "nope")

	_, err := getOSName(missingFile)
	require.Error(t, err)
}
