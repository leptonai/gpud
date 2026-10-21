package firmware

import (
	"context"
	"runtime"
	"testing"
	"testing/fstest"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// gb300HostFS is a host root with the OCI GB300 SMBIOS table and DOCA installed.
func gb300HostFS() fstest.MapFS {
	return fstest.MapFS{
		dmiTablePath: {Data: ociGB300SMBIOS()},
		dpkgStatusPath: {Data: []byte(dpkgStatus(
			dpkgStanza("libc6", "install ok installed", "2.39-0ubuntu8.4"),
			dpkgStanza("doca-ofed", "install ok installed", "3.2.1-044413"),
		))},
	}
}

func gb300GPUs() []fakeGPU {
	return []fakeGPU{
		{busID: "0008:01:00.0", version: "97.10.4A.00.0B", ret: nvml.SUCCESS},
		{busID: "0000:01:00.0", version: "97.10.4A.00.0B", ret: nvml.SUCCESS},
		{busID: "0018:01:00.0", version: "97.10.4A.00.0B", ret: nvml.SUCCESS},
		{busID: "0010:01:00.0", version: "97.10.4A.00.0B", ret: nvml.SUCCESS},
	}
}

func TestCollect_GB300(t *testing.T) {
	rows, err := collect(t.Context(), Options{
		NVMLInstance: newFakeNVML(gb300GPUs()...),
		goos:         "linux",
		goarch:       "arm64",
		hostFS:       gb300HostFS(),
		ibFS:         ibClassFS(ociGB300()...),
		isVM:         notVM,
		now:          func() time.Time { return testReadAt.Add(500 * time.Millisecond) },
	})
	require.NoError(t, err)
	assert.Equal(t, []apiv1.MachineFirmware{
		row(apiv1.FirmwareComponentGPU, "97.10.4A.00.0B", "gpu1", "gpu2", "gpu3", "gpu4"),
		row(apiv1.FirmwareComponentCX8, "40.48.1102", "rdma_rail0", "rdma_rail1", "rdma_rail2", "rdma_rail3"),
		row(apiv1.FirmwareComponentDOCAHost, "3.2.1-044413"),
		row(apiv1.FirmwareComponentHMCSBIOS, "5.36_O72OR37A_02.05.05"),
	}, rows)
}

func TestCollect_NonLinuxReadsOnlyNVML(t *testing.T) {
	host := &openRecorder{FS: gb300HostFS()}
	ib := &openRecorder{FS: ibClassFS(ociGB300()...)}
	rows, err := collect(t.Context(), Options{
		NVMLInstance: newFakeNVML(gb300GPUs()...),
		goos:         "darwin",
		goarch:       "arm64",
		hostFS:       host,
		ibFS:         ib,
		isVM:         notVM,
		now:          func() time.Time { return testReadAt },
	})
	require.NoError(t, err)
	assert.Equal(t, []apiv1.MachineFirmware{
		row(apiv1.FirmwareComponentGPU, "97.10.4A.00.0B", "gpu1", "gpu2", "gpu3", "gpu4"),
	}, rows)
	assert.Empty(t, host.opened)
	assert.Empty(t, ib.opened)
}

func TestCollect_FailedSourcesDoNotHideOthers(t *testing.T) {
	// No NVML, no InfiniBand class directory and an unparsable SMBIOS table:
	// the DOCA version is still reported.
	hostFS := gb300HostFS()
	hostFS[dmiTablePath] = &fstest.MapFile{Data: []byte{0x00, 0x02}}
	rows, err := collect(t.Context(), Options{
		goos:   "linux",
		goarch: "arm64",
		hostFS: hostFS,
		ibFS:   fstest.MapFS{},
		isVM:   notVM,
		now:    func() time.Time { return testReadAt },
	})
	require.NoError(t, err)
	assert.Equal(t, []apiv1.MachineFirmware{
		row(apiv1.FirmwareComponentDOCAHost, "3.2.1-044413"),
	}, rows)
}

func TestCollect_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rows, err := collect(ctx, Options{NVMLInstance: newFakeNVML(gb300GPUs()...), goos: "linux", hostFS: fstest.MapFS{}, ibFS: fstest.MapFS{}})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, rows)
}

func TestOptionsWithDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	assert.Equal(t, runtime.GOOS, o.goos)
	assert.Equal(t, runtime.GOARCH, o.goarch)
	assert.NotNil(t, o.hostFS)
	assert.NotNil(t, o.ibFS)
	assert.NotNil(t, o.isVM)
	assert.NotNil(t, o.now)
	assert.NotPanics(t, func() { _ = o.isVM() })

	// Explicit seams are kept.
	ib := fstest.MapFS{}
	o = Options{goos: "linux", goarch: "arm64", ibFS: ib, InfinibandClassRootDir: "/ignored"}.withDefaults()
	assert.Equal(t, "linux", o.goos)
	assert.Equal(t, "arm64", o.goarch)
	assert.Equal(t, ib, o.ibFS)
}
