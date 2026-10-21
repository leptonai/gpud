package firmware

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

// smbiosTable builds a raw SMBIOS structure table like /sys/firmware/dmi/tables/DMI.
type smbiosTable struct {
	buf    []byte
	handle uint16
}

// add appends one structure. length is the formatted-area length including
// the header; fields maps an offset to a byte value (string indexes, counts).
func (tb *smbiosTable) add(typ byte, length int, fields map[int]byte, strs ...string) *smbiosTable {
	formatted := make([]byte, length)
	formatted[0] = typ
	formatted[1] = byte(length)
	formatted[2] = byte(tb.handle)
	formatted[3] = byte(tb.handle >> 8)
	tb.handle++
	for off, v := range fields {
		formatted[off] = v
	}
	tb.buf = append(tb.buf, formatted...)
	if len(strs) == 0 {
		tb.buf = append(tb.buf, 0, 0)
		return tb
	}
	for _, s := range strs {
		tb.buf = append(tb.buf, s...)
		tb.buf = append(tb.buf, 0)
	}
	tb.buf = append(tb.buf, 0)
	return tb
}

func (tb *smbiosTable) bios(version string) *smbiosTable {
	return tb.add(smbiosTypeBIOS, 0x18, map[int]byte{0x04: 1, 0x05: 2, 0x08: 3}, "NVIDIA", version, "08/01/2026")
}

func (tb *smbiosTable) processor(manufacturer, version string) *smbiosTable {
	return tb.add(smbiosTypeProcessor, 0x30, map[int]byte{0x04: 1, 0x07: 2, 0x10: 3}, "CPU 0", manufacturer, version)
}

// firmwareInventory adds a type 45 entry with one associated component handle (26 bytes).
func (tb *smbiosTable) firmwareInventory(name, version string) *smbiosTable {
	return tb.add(smbiosTypeFirmwareInventory, 0x1A, map[int]byte{0x04: 1, 0x05: 2, 0x0A: 3, 0x17: 1}, name, version, "NVIDIA")
}

func (tb *smbiosTable) end() []byte {
	tb.add(smbiosTypeEndOfTable, smbiosHeaderLen, nil)
	return tb.buf
}

// ociGB300SMBIOS mirrors the OCI GB300 sample: type 0 holds only the short
// BIOS version, type 45 holds the bundle version.
func ociGB300SMBIOS() []byte {
	return (&smbiosTable{}).
		bios("O72OR37A").
		processor("NVIDIA", "Grace A02P").
		processor("NVIDIA", "Grace A02P").
		firmwareInventory("BMC Firmware", "5.01").
		firmwareInventory("BIOS Firmware", "5.36_O72OR37A_02.05.05").
		firmwareInventory("TPM Firmware", "F0017.450000").
		end()
}

// openRecorder fails reads it does not expect, so a test can prove a gate
// skipped the read entirely.
type openRecorder struct {
	fs.FS
	opened []string
}

func (r *openRecorder) Open(name string) (fs.File, error) {
	r.opened = append(r.opened, name)
	return r.FS.Open(name)
}

func dmiFS(table []byte) fstest.MapFS {
	return fstest.MapFS{dmiTablePath: {Data: table}}
}

func notVM() bool { return false }

func TestCollectHostBIOS(t *testing.T) {
	tests := []struct {
		name   string
		table  []byte
		goarch string
		isVM   bool
		want   []instance
	}{
		{
			name:   "OCI GB300 reports the BIOS firmware inventory version",
			table:  ociGB300SMBIOS(),
			goarch: "arm64",
			want:   []instance{{component: apiv1.FirmwareComponentHMCSBIOS, version: "5.36_O72OR37A_02.05.05"}},
		},
		{
			name: "without a BIOS inventory entry the type 0 version is used",
			table: (&smbiosTable{}).
				bios("1.16").
				processor("NVIDIA Corporation", "Grace").
				firmwareInventory("BMC Firmware", "5.01").
				end(),
			goarch: "arm64",
			want:   []instance{{component: apiv1.FirmwareComponentHMCSBIOS, version: "1.16"}},
		},
		{
			name: "UEFI inventory name matches case-insensitively",
			table: (&smbiosTable{}).
				processor("nvidia", "Grace").
				firmwareInventory("System uefi", "02.05.05").
				bios("O72OR37A").
				end(),
			goarch: "arm64",
			want:   []instance{{component: apiv1.FirmwareComponentHMCSBIOS, version: "02.05.05"}},
		},
		{
			name: "BIOS inventory entry with an empty version falls back to type 0",
			table: (&smbiosTable{}).
				bios("O72OR37A").
				processor("NVIDIA", "Grace A02P").
				firmwareInventory("BIOS Firmware", "").
				end(),
			goarch: "arm64",
			want:   []instance{{component: apiv1.FirmwareComponentHMCSBIOS, version: "O72OR37A"}},
		},
		{
			name: "AWS Graviton is not Grace",
			table: (&smbiosTable{}).
				bios("1.0").
				processor("AWS", "AWS Graviton4").
				end(),
			goarch: "arm64",
		},
		{
			name:   "a Grace host without any BIOS version reports nothing",
			table:  (&smbiosTable{}).processor("NVIDIA", "Grace A02P").end(),
			goarch: "arm64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectHostBIOS(dmiFS(tt.table), tt.goarch, func() bool { return tt.isVM })
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollectHostBIOS_GatesSkipTheRead(t *testing.T) {
	t.Run("amd64", func(t *testing.T) {
		rec := &openRecorder{FS: dmiFS(ociGB300SMBIOS())}
		assert.Nil(t, collectHostBIOS(rec, "amd64", notVM))
		assert.Empty(t, rec.opened)
	})
	t.Run("arm64 VM", func(t *testing.T) {
		rec := &openRecorder{FS: dmiFS(ociGB300SMBIOS())}
		assert.Nil(t, collectHostBIOS(rec, "arm64", func() bool { return true }))
		assert.Empty(t, rec.opened)
	})
	t.Run("missing table", func(t *testing.T) {
		assert.Nil(t, collectHostBIOS(fstest.MapFS{}, "arm64", notVM))
	})
}

func TestParseSMBIOS(t *testing.T) {
	ss, err := parseSMBIOS(ociGB300SMBIOS())
	require.NoError(t, err)
	require.Len(t, ss, 7)
	assert.Equal(t, byte(smbiosTypeBIOS), ss[0].typ)
	assert.Equal(t, "O72OR37A", ss[0].str(biosVersionOffset))
	assert.Equal(t, "NVIDIA", ss[1].str(processorManufacturerOffset))
	assert.Equal(t, "BIOS Firmware", ss[4].str(firmwareNameOffset))
	assert.Equal(t, byte(smbiosTypeEndOfTable), ss[6].typ)
	assert.Empty(t, ss[6].strs)

	// Bytes after the end-of-table structure are ignored.
	ss, err = parseSMBIOS(append(ociGB300SMBIOS(), 0xff, 0xff))
	require.NoError(t, err)
	assert.Len(t, ss, 7)

	// A table without the end-of-table structure parses to its end.
	noEnd := (&smbiosTable{}).bios("O72OR37A").buf
	ss, err = parseSMBIOS(noEnd)
	require.NoError(t, err)
	assert.Len(t, ss, 1)

	ss, err = parseSMBIOS(nil)
	require.NoError(t, err)
	assert.Empty(t, ss)
}

func TestParseSMBIOS_Malformed(t *testing.T) {
	valid := (&smbiosTable{}).bios("O72OR37A").buf

	tests := []struct {
		name      string
		table     []byte
		wantCount int
	}{
		{name: "header shorter than four bytes", table: []byte{0x00, 0x18}},
		{name: "length below the header size", table: []byte{0x00, 0x02, 0x00, 0x00, 0x00, 0x00}},
		{name: "formatted area truncated", table: valid[:0x10]},
		{name: "string set without the double NUL", table: valid[:len(valid)-1]},
		{name: "valid structure followed by a truncated one", table: append(append([]byte{}, valid...), 0x04, 0x30, 0x01), wantCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ss, err := parseSMBIOS(tt.table)
			assert.Error(t, err)
			assert.Len(t, ss, tt.wantCount)
		})
	}
}

func TestParseSMBIOS_EveryTruncationIsSafe(t *testing.T) {
	table := ociGB300SMBIOS()
	for n := 0; n <= len(table); n++ {
		ss, _ := parseSMBIOS(table[:n])
		assert.NotPanics(t, func() {
			_ = hasNVIDIAProcessor(ss)
			_ = hostBIOSVersion(ss)
		})
	}
}

func TestSMBIOSStructure_Str(t *testing.T) {
	s := smbiosStructure{
		formatted: []byte{0, 8, 0, 0, 1, 3, 0, 2},
		strs:      []string{" padded ", "second"},
	}
	assert.Equal(t, "padded", s.str(4))
	assert.Equal(t, "", s.str(5), "index past the string set")
	assert.Equal(t, "", s.str(6), "index zero means no string")
	assert.Equal(t, "second", s.str(7))
	assert.Equal(t, "", s.str(8), "offset past the formatted area")
}

func TestHostBIOSVersion_ShortInventoryEntryIgnored(t *testing.T) {
	// A type 45 entry shorter than the specification minimum is not trusted.
	table := (&smbiosTable{}).
		add(smbiosTypeFirmwareInventory, 0x10, map[int]byte{0x04: 1, 0x05: 2}, "BIOS Firmware", "bogus").
		bios("O72OR37A").
		end()
	ss, err := parseSMBIOS(table)
	require.NoError(t, err)
	assert.Equal(t, "O72OR37A", hostBIOSVersion(ss))
}

func FuzzParseSMBIOS(f *testing.F) {
	f.Add(ociGB300SMBIOS())
	f.Add([]byte{})
	f.Add([]byte{0x2d, 0x04, 0x00})
	f.Fuzz(func(t *testing.T, b []byte) {
		ss, _ := parseSMBIOS(b)
		_ = hasNVIDIAProcessor(ss)
		_ = hostBIOSVersion(ss)
	})
}
