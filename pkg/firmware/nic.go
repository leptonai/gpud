package firmware

import (
	"errors"
	"io/fs"
	"path"
	"sort"
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

// nicFunction is one ConnectX-8 or BlueField-3 PF in the class directory.
type nicFunction struct {
	name     string
	deviceID uint64
	// awsPSID is set when the PSID (board_id) marks a ConnectX-8 built for AWS.
	awsPSID bool
	version string
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

	var fns []nicFunction
	for _, entry := range entries {
		// Entries are symlinks into /sys/devices, so no IsDir filter.
		if fn, ok := readNICFunction(ibFS, entry.Name()); ok {
			fns = append(fns, fn)
		}
	}

	cards := groupCards(fns)
	found := make([]instance, 0, len(cards))
	for _, pfs := range cards {
		// The card is named after its first PF and reports the version of its
		// first PF that has one. Any PF with an AWS PSID marks the whole card,
		// so a PF missing board_id or fw_ver cannot split or misclassify it.
		sort.Slice(pfs, func(i, j int) bool { return lowerFunction(pfs[i], pfs[j]) })
		var version string
		aws := false
		for _, fn := range pfs {
			if version == "" {
				version = fn.version
			}
			aws = aws || fn.awsPSID
		}
		if version == "" {
			log.Logger.Debugw("skipping nic firmware", "device", pfs[0].name, "reason", "no fw_ver")
			continue
		}

		component := apiv1.FirmwareComponentBF3NIC
		if pfs[0].deviceID == pciDeviceCX8 {
			component = apiv1.FirmwareComponentCX8
			if aws {
				component = apiv1.FirmwareComponentCX8AWS
			}
		}
		found = append(found, instance{component: component, version: version, name: pfs[0].name})
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

	if deviceID != pciDeviceCX8 && deviceID != pciDeviceBF3 {
		return nicFunction{}, false
	}

	// A PF without fw_ver is kept: its PSID and keys still classify and join
	// its card.
	return nicFunction{
		name:     name,
		deviceID: deviceID,
		awsPSID:  deviceID == pciDeviceCX8 && strings.HasPrefix(readTrimmed(ibFS, path.Join(name, "board_id")), awsPSIDPrefix),
		version:  readTrimmed(ibFS, path.Join(name, "fw_ver")),
		slot:     uevent["PCI_SLOT_NAME"],
		guid:     readTrimmed(ibFS, path.Join(name, "sys_image_guid")),
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

// groupCards returns the PFs of each physical card. Two PFs belong to one card
// when they share either key: sys_image_guid, which every function of a card
// reports, including socket-direct PFs on different PCI domains, or the PCI
// slot without its function number. A PF missing one key still joins its card
// through the other; a PF with neither is a card of its own.
func groupCards(fns []nicFunction) [][]nicFunction {
	parent := make([]int, len(fns))
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}

	owner := make(map[string]int)
	for i, fn := range fns {
		parent[i] = i
		for _, key := range cardKeys(fn) {
			if j, ok := owner[key]; ok {
				parent[find(i)] = find(j)
			} else {
				owner[key] = i
			}
		}
	}

	byRoot := make(map[int][]nicFunction)
	for i, fn := range fns {
		root := find(i)
		byRoot[root] = append(byRoot[root], fn)
	}
	cards := make([][]nicFunction, 0, len(byRoot))
	for _, pfs := range byRoot {
		cards = append(cards, pfs)
	}
	return cards
}

// cardKeys returns the identities a PF shares with the other PFs of its card,
// scoped to its device ID.
func cardKeys(fn nicFunction) []string {
	prefix := strconv.FormatUint(fn.deviceID, 16) + "|"
	var keys []string
	if fn.guid != "" && fn.guid != zeroGUID {
		keys = append(keys, prefix+"guid:"+fn.guid)
	}
	if fn.slot != "" {
		slot := fn.slot
		if idx := strings.LastIndexByte(slot, '.'); idx >= 0 {
			slot = slot[:idx]
		}
		keys = append(keys, prefix+"slot:"+slot)
	}
	return keys
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
