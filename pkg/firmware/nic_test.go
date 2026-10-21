package firmware

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// ibDevice is one /sys/class/infiniband/<name> entry. Empty fields are not
// written, which models a missing sysfs attribute.
type ibDevice struct {
	name    string
	hcaType string
	boardID string
	fwVer   string
	guid    string
	pciID   string
	slot    string
	vf      bool
}

func (d ibDevice) files() map[string]string {
	files := map[string]string{}
	put := func(rel, v string) {
		if v != "" {
			files[rel] = v + "\n"
		}
	}
	put("hca_type", d.hcaType)
	put("board_id", d.boardID)
	put("fw_ver", d.fwVer)
	put("sys_image_guid", d.guid)
	if d.pciID != "" || d.slot != "" {
		files["device/uevent"] = fmt.Sprintf("DRIVER=mlx5_core\nPCI_CLASS=20000\nPCI_ID=%s\nPCI_SUBSYS_ID=15B3:0071\nPCI_SLOT_NAME=%s\n", d.pciID, d.slot)
	}
	if d.vf {
		files["device/physfn/uevent"] = "DRIVER=mlx5_core\n"
	}
	return files
}

func ibClassFS(devs ...ibDevice) fstest.MapFS {
	fsys := fstest.MapFS{}
	for _, d := range devs {
		for rel, data := range d.files() {
			fsys[d.name+"/"+rel] = &fstest.MapFile{Data: []byte(data)}
		}
	}
	return fsys
}

// ociGB300 models an OCI GB300 host: four CX8 rails on four PCI domains with
// distinct sys_image_guids, one VF per rail carrying its PF's guid, and a
// socket-direct CX7 whose two PFs share one guid.
func ociGB300() []ibDevice {
	devs := []ibDevice{
		{name: "rdma_rail0", hcaType: "MT4131", boardID: "MT_0000001513", fwVer: "40.48.1102", guid: "90e3:1703:00be:ed38", pciID: "15B3:1023", slot: "0000:03:00.0"},
		{name: "rdma_rail1", hcaType: "MT4131", boardID: "MT_0000001513", fwVer: "40.48.1102", guid: "90e3:1703:00be:ed58", pciID: "15B3:1023", slot: "0002:03:00.0"},
		{name: "rdma_rail2", hcaType: "MT4131", boardID: "MT_0000001513", fwVer: "40.48.1102", guid: "90e3:1703:00d0:9e32", pciID: "15B3:1023", slot: "0010:03:00.0"},
		{name: "rdma_rail3", hcaType: "MT4131", boardID: "MT_0000001513", fwVer: "40.48.1102", guid: "90e3:1703:00d0:9e52", pciID: "15B3:1023", slot: "0012:03:00.0"},
		{name: "mlx5_8", hcaType: "MT4129", boardID: "ORC0000000014", fwVer: "28.43.1014", guid: "549b:2403:0079:ca9e", pciID: "15B3:1021", slot: "0007:01:00.0"},
		{name: "mlx5_17", hcaType: "MT4129", boardID: "ORC0000000014", fwVer: "28.43.1014", guid: "549b:2403:0079:ca9e", pciID: "15B3:1021", slot: "0017:01:00.0"},
	}
	for i, guid := range []string{"90e3:1703:00be:ed38", "90e3:1703:00be:ed58", "90e3:1703:00d0:9e32", "90e3:1703:00d0:9e52"} {
		devs = append(devs, ibDevice{
			name: fmt.Sprintf("rdma_vf_rail%d", i), hcaType: "MT4126", boardID: "MT_0000001513", fwVer: "40.48.1102",
			guid: guid, pciID: "15B3:101E", slot: fmt.Sprintf("00%02d:03:00.2", []int{0, 2, 10, 12}[i]), vf: true,
		})
	}
	return devs
}

func collectNICRows(t *testing.T, devs ...ibDevice) []apiv1.MachineFirmware {
	t.Helper()
	return group(collectNICs(ibClassFS(devs...)), testReadAt)
}

var testReadAt = time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)

func row(component, version string, names ...string) apiv1.MachineFirmware {
	r := apiv1.MachineFirmware{
		Component:   component,
		Version:     version,
		Instances:   int32(len(names)),
		LastUpdated: metav1.NewTime(testReadAt),
	}
	if len(names) > 0 {
		r.InstanceNames = names
	} else {
		r.Instances = 1
	}
	return r
}

func TestCollectNICs_OCIGB300(t *testing.T) {
	assert.Equal(t, []apiv1.MachineFirmware{
		row(apiv1.FirmwareComponentCX8, "40.48.1102", "rdma_rail0", "rdma_rail1", "rdma_rail2", "rdma_rail3"),
	}, collectNICRows(t, ociGB300()...))
}

func TestCollectNICs_H100BF3BondFixture(t *testing.T) {
	// GPUd's own H100 fixture: eight CX7 HCAs (ignored) and a BF3 bond device
	// with only fw_ver, board_id and hca_type.
	root := filepath.Join("..", "..", "components", "accelerator", "nvidia", "infiniband", "class", "testdata", "sys-class-infiniband-h100.0")
	rows := group(collectNICs(os.DirFS(root)), testReadAt)
	assert.Equal(t, []apiv1.MachineFirmware{
		row(apiv1.FirmwareComponentBF3NIC, "32.38.3056", "mlx5_bond_0"),
	}, rows)
}

func TestCollectNICs_Classification(t *testing.T) {
	tests := []struct {
		name string
		devs []ibDevice
		want []apiv1.MachineFirmware
	}{
		{
			name: "cx8 with an AWS PSID",
			devs: []ibDevice{
				{name: "mlx5_0", hcaType: "MT4131", boardID: "AS00000000013", fwVer: "40.47.2526", guid: "a088:c203:0001:0001", slot: "0000:05:00.0"},
				{name: "mlx5_1", hcaType: "MT4131", boardID: "MT_0000001513", fwVer: "40.47.2526", guid: "a088:c203:0001:0002", slot: "0000:06:00.0"},
				{name: "mlx5_2", hcaType: "MT4131", boardID: "DEL0000000099", fwVer: "40.47.2526", guid: "a088:c203:0001:0003", slot: "0000:07:00.0"},
			},
			want: []apiv1.MachineFirmware{
				row(apiv1.FirmwareComponentCX8, "40.47.2526", "mlx5_1", "mlx5_2"),
				row(apiv1.FirmwareComponentCX8AWS, "40.47.2526", "mlx5_0"),
			},
		},
		{
			name: "mixed versions across cards give one row each",
			devs: []ibDevice{
				{name: "mlx5_0", hcaType: "MT4131", fwVer: "40.47.2526", guid: "0000:0000:0000:0a01", slot: "0000:01:00.0"},
				{name: "mlx5_1", hcaType: "MT4131", fwVer: "40.48.1102", guid: "0000:0000:0000:0a02", slot: "0000:02:00.0"},
				{name: "mlx5_10", hcaType: "MT4131", fwVer: "40.47.2526", guid: "0000:0000:0000:0a03", slot: "0000:03:00.0"},
				{name: "mlx5_9", hcaType: "MT4131", fwVer: "40.47.2526", guid: "0000:0000:0000:0a04", slot: "0000:04:00.0"},
			},
			want: []apiv1.MachineFirmware{
				row(apiv1.FirmwareComponentCX8, "40.47.2526", "mlx5_0", "mlx5_9", "mlx5_10"),
				row(apiv1.FirmwareComponentCX8, "40.48.1102", "mlx5_1"),
			},
		},
		{
			name: "one card with four PFs is named after its lowest PCI function",
			devs: []ibDevice{
				{name: "mlx5_a", hcaType: "MT4131", fwVer: "40.48.1102", guid: "90e3:1703:00be:ed38", slot: "0000:03:00.3"},
				{name: "mlx5_b", hcaType: "MT4131", fwVer: "40.48.1102", guid: "90e3:1703:00be:ed38", slot: "0000:03:00.1"},
				{name: "mlx5_c", hcaType: "MT4131", fwVer: "40.48.1102", guid: "90e3:1703:00be:ed38", slot: "0000:03:00.0"},
				{name: "mlx5_d", hcaType: "MT4131", fwVer: "40.48.1102", guid: "90e3:1703:00be:ed38", slot: "0000:03:00.2"},
			},
			want: []apiv1.MachineFirmware{
				row(apiv1.FirmwareComponentCX8, "40.48.1102", "mlx5_c"),
			},
		},
		{
			name: "bf3 with two PFs sharing a guid is one card",
			devs: []ibDevice{
				{name: "mlx5_0", hcaType: "MT41692", boardID: "MT_0000000884", fwVer: "32.47.2526", guid: "b8e9:2403:0011:2233", pciID: "15B3:A2DC", slot: "0000:c1:00.0"},
				{name: "mlx5_1", hcaType: "MT41692", boardID: "MT_0000000884", fwVer: "32.47.2526", guid: "b8e9:2403:0011:2233", pciID: "15B3:A2DC", slot: "0000:c1:00.1"},
			},
			want: []apiv1.MachineFirmware{
				row(apiv1.FirmwareComponentBF3NIC, "32.47.2526", "mlx5_0"),
			},
		},
		{
			name: "zero or missing guid falls back to the PCI slot without the function",
			devs: []ibDevice{
				{name: "mlx5_0", hcaType: "MT4131", fwVer: "40.48.1102", guid: zeroGUID, slot: "0000:41:00.0"},
				{name: "mlx5_1", hcaType: "MT4131", fwVer: "40.48.1102", guid: zeroGUID, slot: "0000:41:00.1"},
				{name: "mlx5_2", hcaType: "MT4131", fwVer: "40.48.1102", slot: "0000:42:00.0"},
				{name: "mlx5_3", hcaType: "MT4131", fwVer: "40.48.1102", slot: "0000:42:00.1"},
			},
			want: []apiv1.MachineFirmware{
				row(apiv1.FirmwareComponentCX8, "40.48.1102", "mlx5_0", "mlx5_2"),
			},
		},
		{
			name: "no guid and no slot counts each device",
			devs: []ibDevice{
				{name: "mlx5_bond_0", hcaType: "MT41692", fwVer: "32.38.3056"},
				{name: "mlx5_bond_1", hcaType: "MT41692", fwVer: "32.38.3056"},
			},
			want: []apiv1.MachineFirmware{
				row(apiv1.FirmwareComponentBF3NIC, "32.38.3056", "mlx5_bond_0", "mlx5_bond_1"),
			},
		},
		{
			name: "hca_type missing falls back to the uevent PCI ID",
			devs: []ibDevice{
				{name: "mlx5_0", fwVer: "40.48.1102", pciID: "15b3:1023", slot: "0000:03:00.0"},
				{name: "mlx5_1", fwVer: "32.47.2526", pciID: "15B3:A2DC", slot: "0000:04:00.0"},
			},
			want: []apiv1.MachineFirmware{
				row(apiv1.FirmwareComponentBF3NIC, "32.47.2526", "mlx5_1"),
				row(apiv1.FirmwareComponentCX8, "40.48.1102", "mlx5_0"),
			},
		},
		{
			name: "AWS EFA, CX7, BF3 Lx and VFs are ignored",
			devs: []ibDevice{
				{name: "rdmap40s0", fwVer: "0.0.0.0", guid: zeroGUID, pciID: "1D0F:EFA3", slot: "0000:28:00.0"},
				{name: "mlx5_0", hcaType: "MT4129", fwVer: "28.43.1014", slot: "0000:1a:00.0"},
				{name: "mlx5_1", hcaType: "MT41689", fwVer: "32.47.2526", slot: "0000:1b:00.0"},
				{name: "mlx5_2", hcaType: "MT4126", fwVer: "40.48.1102", slot: "0000:03:00.2", vf: true},
				// A VF is skipped by device/physfn even when it reports the PF device ID.
				{name: "mlx5_3", hcaType: "MT4131", fwVer: "40.48.1102", slot: "0000:03:00.3", vf: true},
			},
			want: nil,
		},
		{
			name: "a device without fw_ver is skipped and the others are kept",
			devs: []ibDevice{
				{name: "mlx5_0", hcaType: "MT4131", guid: "0000:0000:0000:0001", slot: "0000:01:00.0"},
				{name: "mlx5_1", hcaType: "MT4131", fwVer: "40.48.1102", guid: "0000:0000:0000:0002", slot: "0000:02:00.0"},
			},
			want: []apiv1.MachineFirmware{
				row(apiv1.FirmwareComponentCX8, "40.48.1102", "mlx5_1"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, collectNICRows(t, tt.devs...))
		})
	}
}

func TestCollectNICs_MissingOrEmptyRoot(t *testing.T) {
	assert.Empty(t, collectNICs(os.DirFS(filepath.Join(t.TempDir(), "missing"))))
	assert.Empty(t, collectNICs(os.DirFS(t.TempDir())))
	assert.Empty(t, collectNICs(fstest.MapFS{}))
}

func TestCollectNICs_FollowsSysfsSymlinks(t *testing.T) {
	// /sys/class/infiniband entries are symlinks into /sys/devices, and a VF's
	// device/physfn is a symlink to its PF.
	tmp := t.TempDir()
	pf := filepath.Join(tmp, "devices", "pci0000:00", "0000:03:00.0")
	vf := filepath.Join(tmp, "devices", "pci0000:00", "0000:03:00.2")
	pfIB := filepath.Join(pf, "infiniband", "rdma_rail0")
	vfIB := filepath.Join(vf, "infiniband", "rdma_vf_rail0")
	for _, dir := range []string{pfIB, vfIB} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	write := func(path, data string) {
		require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
	}
	write(filepath.Join(pf, "uevent"), "PCI_ID=15B3:1023\nPCI_SLOT_NAME=0000:03:00.0\n")
	write(filepath.Join(vf, "uevent"), "PCI_ID=15B3:101E\nPCI_SLOT_NAME=0000:03:00.2\n")
	require.NoError(t, os.Symlink(pf, filepath.Join(vf, "physfn")))
	for _, ib := range []struct{ dir, dev string }{{pfIB, pf}, {vfIB, vf}} {
		write(filepath.Join(ib.dir, "hca_type"), "MT4131\n")
		write(filepath.Join(ib.dir, "fw_ver"), "40.48.1102\n")
		write(filepath.Join(ib.dir, "sys_image_guid"), "90e3:1703:00be:ed38\n")
		require.NoError(t, os.Symlink(ib.dev, filepath.Join(ib.dir, "device")))
	}

	class := filepath.Join(tmp, "class", "infiniband")
	require.NoError(t, os.MkdirAll(class, 0o755))
	require.NoError(t, os.Symlink(pfIB, filepath.Join(class, "rdma_rail0")))
	require.NoError(t, os.Symlink(vfIB, filepath.Join(class, "rdma_vf_rail0")))

	rows, err := collect(t.Context(), Options{
		InfinibandClassRootDir: class,
		goos:                   "linux",
		goarch:                 "amd64",
		hostFS:                 fstest.MapFS{},
		now:                    func() time.Time { return testReadAt },
	})
	require.NoError(t, err)
	assert.Equal(t, []apiv1.MachineFirmware{
		row(apiv1.FirmwareComponentCX8, "40.48.1102", "rdma_rail0"),
	}, rows)
}

func TestPCIDeviceID(t *testing.T) {
	tests := []struct {
		hcaType string
		pciID   string
		want    uint64
		ok      bool
	}{
		{hcaType: "MT4131", want: pciDeviceCX8, ok: true},
		{hcaType: "MT41692", pciID: "15B3:1023", want: pciDeviceBF3, ok: true},
		{pciID: "15B3:1023", want: pciDeviceCX8, ok: true},
		{pciID: "15b3:a2dc", want: pciDeviceBF3, ok: true},
		{hcaType: "MTx", pciID: "15B3:A2DC", want: pciDeviceBF3, ok: true},
		{pciID: "1D0F:EFA3"},
		{pciID: "15B3:zz"},
		{pciID: "garbage"},
		{hcaType: "MT"},
		{},
	}
	for _, tt := range tests {
		t.Run(tt.hcaType+"/"+tt.pciID, func(t *testing.T) {
			got, ok := pciDeviceID(tt.hcaType, tt.pciID)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLowerFunction(t *testing.T) {
	a := nicFunction{name: "mlx5_1", slot: "0000:03:00.0"}
	b := nicFunction{name: "mlx5_0", slot: "0000:03:00.1"}
	noSlot := nicFunction{name: "mlx5_bond_0"}
	assert.True(t, lowerFunction(a, b))
	assert.False(t, lowerFunction(b, a))
	assert.True(t, lowerFunction(a, noSlot))
	assert.False(t, lowerFunction(noSlot, a))
	assert.True(t, lowerFunction(nicFunction{name: "mlx5_2"}, nicFunction{name: "mlx5_10"}))
}
