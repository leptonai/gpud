// Package firmware reads the firmware and host software versions that operators
// compare against a release bundle: GPU VBIOS, ConnectX-8 and BlueField-3 NIC
// firmware, the Grace host BIOS and the DOCA host package.
//
// Every source is read without exec. A source that fails is skipped so it
// never hides the versions read from the other sources.
package firmware

import (
	"context"
	"io/fs"
	"os"
	"runtime"
	"time"

	apiv1 "github.com/leptonai/gpud/api/v1"
	infinibandclass "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/class"
	pkghost "github.com/leptonai/gpud/pkg/host"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
)

// DefaultCollectInterval is how often a running GPUd re-reads firmware versions.
// A new version shows up only after a flash plus a reset or reboot, so a slow
// refresh is enough.
const DefaultCollectInterval = 4 * time.Hour

// Paths relative to the host root.
const (
	dmiTablePath   = "sys/firmware/dmi/tables/DMI"
	dpkgStatusPath = "var/lib/dpkg/status"
)

// Options selects the inputs of one collection.
type Options struct {
	// NVMLInstance reads the GPU VBIOS versions.
	// Nil or a NoOp instance yields no GPU entries; the other sources still run.
	NVMLInstance nvidianvml.Instance
	// InfinibandClassRootDir overrides the InfiniBand class directory
	// (default /sys/class/infiniband), matching the infiniband component.
	InfinibandClassRootDir string

	// Test seams. Zero values select the running host.
	goos   string
	goarch string
	hostFS fs.FS
	ibFS   fs.FS
	isVM   func() bool
	now    func() time.Time
}

func (o Options) withDefaults() Options {
	if o.goos == "" {
		o.goos = runtime.GOOS
	}
	if o.goarch == "" {
		o.goarch = runtime.GOARCH
	}
	if o.hostFS == nil {
		o.hostFS = os.DirFS("/")
	}
	if o.ibFS == nil {
		root := o.InfinibandClassRootDir
		if root == "" {
			root = infinibandclass.DefaultRootDir
		}
		// os.DirFS follows the symlinks that every /sys/class/infiniband entry is;
		// os.Root would refuse them because they point into /sys/devices.
		o.ibFS = os.DirFS(root)
	}
	if o.isVM == nil {
		o.isVM = hostIsVM
	}
	if o.now == nil {
		o.now = time.Now
	}
	return o
}

// hostIsVM treats an unknown virtualization state as a VM: the detector leaves
// VM empty when systemd-detect-virt is missing or fails, and only a confirmed
// bare-metal host may report its BIOS.
func hostIsVM() bool {
	return pkghost.VirtualizationEnv().VM != "none"
}

// collect reads every source once. It returns the context error when ctx is
// done before the collection finished, so neither a partial result nor one
// that outlived its caller is published.
func collect(ctx context.Context, opts Options) ([]apiv1.MachineFirmware, error) {
	o := opts.withDefaults()
	readAt := o.now()

	sources := []func() []instance{
		func() []instance { return collectGPU(o.NVMLInstance) },
	}
	// The remaining sources read Linux sysfs and dpkg files.
	if o.goos == "linux" {
		sources = append(sources,
			func() []instance { return collectNICs(o.ibFS) },
			func() []instance { return collectHostBIOS(o.hostFS, o.goarch, o.isVM) },
			func() []instance { return collectDOCAHost(o.hostFS) },
		)
	}

	var found []instance
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		found = append(found, source()...)
	}
	// NVML reads ignore ctx, so the last source can return after it is done.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return group(found, readAt), nil
}
