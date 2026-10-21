package firmware

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/log"
)

// SMBIOS structure types and field offsets (DMTF DSP0134).
const (
	smbiosTypeBIOS              = 0
	smbiosTypeProcessor         = 4
	smbiosTypeFirmwareInventory = 45
	smbiosTypeEndOfTable        = 127

	smbiosHeaderLen = 4

	biosVersionOffset           = 0x05
	processorManufacturerOffset = 0x07
	firmwareNameOffset          = 0x04
	firmwareVersionOffset       = 0x05
	firmwareInventoryMinLen     = 0x18
)

// biosFirmwareName matches the whole name of the host BIOS inventory entry
// ("BIOS Firmware", "System UEFI"), so peripheral entries such as "GPU VBIOS"
// or "NIC UEFI" never stand in for it.
var biosFirmwareName = regexp.MustCompile(`(?i)^(system\s+)?(s?bios|uefi)(\s+firmware)?$`)

// smbiosStructure is one structure of a raw SMBIOS table.
type smbiosStructure struct {
	typ byte
	// formatted is the formatted area, including the 4-byte header, so the
	// offsets match the specification.
	formatted []byte
	strs      []string
}

// str returns the string that the byte at offset points to, or "" when the
// structure is too short or the index is unset or out of range.
func (s smbiosStructure) str(offset int) string {
	if offset >= len(s.formatted) {
		return ""
	}
	idx := int(s.formatted[offset])
	if idx == 0 || idx > len(s.strs) {
		return ""
	}
	return strings.TrimSpace(s.strs[idx-1])
}

// parseSMBIOS walks a raw SMBIOS structure table such as
// /sys/firmware/dmi/tables/DMI. It stops at the end-of-table structure and
// returns the structures parsed so far together with an error when the table
// is malformed or truncated.
func parseSMBIOS(b []byte) ([]smbiosStructure, error) {
	var out []smbiosStructure
	off := 0
	for off < len(b) {
		if len(b)-off < smbiosHeaderLen {
			return out, fmt.Errorf("truncated smbios header at offset %d", off)
		}
		typ := b[off]
		length := int(b[off+1])
		if length < smbiosHeaderLen {
			return out, fmt.Errorf("invalid smbios structure length %d at offset %d", length, off)
		}
		if off+length > len(b) {
			return out, fmt.Errorf("truncated smbios structure at offset %d", off)
		}

		// The string set follows the formatted area and ends with two NULs.
		strStart := off + length
		end := bytes.Index(b[strStart:], []byte{0, 0})
		if end < 0 {
			return out, fmt.Errorf("unterminated smbios string set at offset %d", off)
		}
		var strs []string
		if end > 0 {
			strs = strings.Split(string(b[strStart:strStart+end]), "\x00")
		}

		out = append(out, smbiosStructure{typ: typ, formatted: b[off : off+length], strs: strs})
		if typ == smbiosTypeEndOfTable {
			return out, nil
		}
		off = strStart + end + 2
	}
	return out, nil
}

// hasNVIDIAProcessor reports whether any processor structure names NVIDIA as
// the manufacturer, which is true on Grace and false on Graviton or x86.
func hasNVIDIAProcessor(ss []smbiosStructure) bool {
	for _, s := range ss {
		if s.typ == smbiosTypeProcessor && strings.Contains(strings.ToLower(s.str(processorManufacturerOffset)), "nvidia") {
			return true
		}
	}
	return false
}

// hostBIOSVersion prefers the firmware inventory entry for the BIOS, whose
// version carries the bundle version (OCI GB300: "5.36_O72OR37A_02.05.05"),
// over the shorter type 0 BIOS version ("O72OR37A").
func hostBIOSVersion(ss []smbiosStructure) string {
	for _, s := range ss {
		if s.typ != smbiosTypeFirmwareInventory || len(s.formatted) < firmwareInventoryMinLen {
			continue
		}
		if !biosFirmwareName.MatchString(s.str(firmwareNameOffset)) {
			continue
		}
		if v := s.str(firmwareVersionOffset); v != "" {
			return v
		}
	}
	for _, s := range ss {
		if s.typ == smbiosTypeBIOS {
			if v := s.str(biosVersionOffset); v != "" {
				return v
			}
		}
	}
	return ""
}

// collectHostBIOS reports the system BIOS of a bare-metal NVIDIA Grace host.
// Other hosts are skipped: their BIOS is not part of the firmware bundle, and
// a VM sees the hypervisor's virtual BIOS.
func collectHostBIOS(hostFS fs.FS, goarch string, isVM func() bool) []instance {
	if goarch != "arm64" || isVM() {
		return nil
	}

	// Parse the raw table: it needs no dmidecode exec and no dmi_sysfs module.
	b, err := fs.ReadFile(hostFS, dmiTablePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Logger.Debugw("skipping host bios firmware", "error", err)
		}
		return nil
	}
	ss, err := parseSMBIOS(b)
	if err != nil {
		log.Logger.Debugw("partially parsed smbios table", "error", err)
	}
	if !hasNVIDIAProcessor(ss) {
		return nil
	}

	version := hostBIOSVersion(ss)
	if version == "" {
		return nil
	}
	return []instance{{component: apiv1.FirmwareComponentHMCSBIOS, version: version}}
}
