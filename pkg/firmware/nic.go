package firmware

import (
	"errors"
	"io/fs"
	"path"
	"strconv"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

const (
	mellanoxVendorID = 0x15b3

	// mlx5 reports hca_type as "MT" plus the decimal PCI device ID.
	pciDeviceCX8 = 0x1023 // MT4131
	pciDeviceBF3 = 0xa2dc // MT41692, BlueField-3 integrated ConnectX-7

	// PSID (board_id) prefix of the ConnectX-8 cards built for AWS.
	awsPSIDPrefix = "AS"

	zeroGUID = "0000:0000:0000:0000"
)

// nicFunction is one InfiniBand device in the class directory that maps to a
// reported component.
type nicFunction struct {
	name      string
	component string
	version   string
	// slot is the PCI address (PCI_SLOT_NAME), for example "0000:03:00.0".
	slot string
	guid string
}

// collectNICs reports one instance per physical ConnectX-8 or BlueField-3 card.
//
// It does not reuse infiniband/class.LoadDevices: that skips names without the
// "mlx" prefix (OCI GB300 rails are rdma_rail*), reads every port counter, and
// fails the whole load on one bad device. It also does not apply
// ExcludedInfinibandDevices: the files read here are attributes cached by the
// driver, not the counter reads that trigger ACCESS_REG errors on restricted PFs.
func collectNICs(ibFS fs.FS) []instance {
	entries, err := fs.ReadDir(ibFS, ".")
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Logger.Debugw("skipping nic firmware", "error", err)
		}
		return nil
	}

	cards := make(map[string]nicFunction)
	for _, entry := range entries {
		// Entries are symlinks into /sys/devices, so no IsDir filter.
		fn, ok := readNICFunction(ibFS, entry.Name())
		if !ok {
			continue
		}
		key := fn.component + "|" + cardKey(fn)
		if cur, exists := cards[key]; !exists || lowerFunction(fn, cur) {
			cards[key] = fn
		}
	}

	found := make([]instance, 0, len(cards))
	for _, fn := range cards {
		found = append(found, instance{component: fn.component, version: fn.version, name: fn.name})
	}
	return found
}

func readNICFunction(ibFS fs.FS, name string) (nicFunction, bool) {
	// Only SR-IOV VFs have device/physfn; they repeat their PF's firmware.
	if _, err := fs.Stat(ibFS, path.Join(name, "device", "physfn")); err == nil {
		return nicFunction{}, false
	}

	uevent := parseUevent(readTrimmed(ibFS, path.Join(name, "device", "uevent")))
	deviceID, ok := pciDeviceID(readTrimmed(ibFS, path.Join(name, "hca_type")), uevent["PCI_ID"])
	if !ok {
		return nicFunction{}, false
	}

	var component string
	switch deviceID {
	case pciDeviceCX8:
		component = apiv1.FirmwareComponentCX8
		if strings.HasPrefix(readTrimmed(ibFS, path.Join(name, "board_id")), awsPSIDPrefix) {
			component = apiv1.FirmwareComponentCX8AWS
		}
	case pciDeviceBF3:
		component = apiv1.FirmwareComponentBF3NIC
	default:
		return nicFunction{}, false
	}

	version := readTrimmed(ibFS, path.Join(name, "fw_ver"))
	if version == "" {
		log.Logger.Debugw("skipping nic firmware", "device", name, "reason", "no fw_ver")
		return nicFunction{}, false
	}

	return nicFunction{
		name:      name,
		component: component,
		version:   version,
		slot:      uevent["PCI_SLOT_NAME"],
		guid:      readTrimmed(ibFS, path.Join(name, "sys_image_guid")),
	}, true
}

// pciDeviceID returns the Mellanox PCI device ID from hca_type ("MT4131"),
// falling back to the uevent PCI_ID ("15B3:1023").
func pciDeviceID(hcaType, pciID string) (uint64, bool) {
	if digits, ok := strings.CutPrefix(hcaType, "MT"); ok {
		if id, err := strconv.ParseUint(digits, 10, 32); err == nil {
			return id, true
		}
	}

	vendor, dev, ok := strings.Cut(pciID, ":")
	if !ok {
		return 0, false
	}
	vendorID, err := strconv.ParseUint(vendor, 16, 16)
	if err != nil || vendorID != mellanoxVendorID {
		return 0, false
	}
	id, err := strconv.ParseUint(dev, 16, 16)
	if err != nil {
		return 0, false
	}
	return id, true
}

// cardKey groups the PFs of one physical card. sys_image_guid is shared by
// every function of a card, including socket-direct PFs on different PCI
// domains; the PCI slot without its function number is the fallback.
func cardKey(fn nicFunction) string {
	if fn.guid != "" && fn.guid != zeroGUID {
		return "guid:" + fn.guid
	}
	if fn.slot != "" {
		slot := fn.slot
		if idx := strings.LastIndexByte(slot, '.'); idx >= 0 {
			slot = slot[:idx]
		}
		return "slot:" + slot
	}
	return "name:" + fn.name
}

// lowerFunction reports whether a has a lower PCI address than b, so the card
// is named after its first PF.
func lowerFunction(a, b nicFunction) bool {
	if a.slot != b.slot {
		if a.slot == "" || b.slot == "" {
			return b.slot == ""
		}
		return a.slot < b.slot
	}
	return naturalLess(a.name, b.name)
}

func parseUevent(s string) map[string]string {
	kv := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			kv[k] = v
		}
	}
	return kv
}

func readTrimmed(fsys fs.FS, name string) string {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
