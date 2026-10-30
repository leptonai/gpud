package memory

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	memorysignals "github.com/dsx-ai-factory/health-validation/collect/signals/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_readBPFJITBufferBytes_Fixture(t *testing.T) {
	data, err := os.ReadFile("testdata/vmallocinfo.bpf_jit_alloc_exec")
	require.NoError(t, err)
	files := fstest.MapFS{"proc/vmallocinfo": &fstest.MapFile{Data: data}}

	size, err := readBPFJITBufferBytes(files)
	require.NoError(t, err)
	assert.Equal(t, uint64(3977216), size)
}

func Test_readBPFJITBufferBytes(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want uint64
		err  string
	}{
		{name: "empty"},
		{name: "unrelated allocation", data: "0x0-0x1 16384 other_function pages=3"},
		{name: "sum", data: "0x0-0x1 16384 bpf_jit_alloc_exec pages=3\n0x1-0x2 4096 bpf_jit_alloc_exec pages=1", want: 20480},
		{name: "invalid size", data: "0x0-0x1 invalid bpf_jit_alloc_exec", err: "PARSE_ERROR"},
		{name: "missing size", data: "bpf_jit_alloc_exec", err: "PARSE_ERROR"},
		{name: "negative size", data: "0x0-0x1 -1 bpf_jit_alloc_exec", err: "PARSE_ERROR"},
		{name: "overflow", data: "0x0-0x1 18446744073709551615 bpf_jit_alloc_exec\n0x1-0x2 1 bpf_jit_alloc_exec", err: "overflow"},
		{name: "long unrelated line before allocation", data: strings.Repeat("x", 65537) + "\n0x0-0x1 4096 bpf_jit_alloc_exec", want: 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// No meminfo fixture: selecting BPF must not read other memory signals.
			files := fstest.MapFS{"proc/vmallocinfo": &fstest.MapFile{Data: []byte(tc.data)}}
			size, err := readBPFJITBufferBytes(files)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				assert.Zero(t, size)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, size)
		})
	}
}

func Test_readBPFJITBufferBytes_ReadFailure(t *testing.T) {
	_, err := readBPFJITBufferBytes(fstest.MapFS{})
	require.ErrorContains(t, err, "UNAVAILABLE")

	_, err = readBPFJITBufferBytes(deniedBPFFile{})
	require.ErrorContains(t, err, "PERMISSION_DENIED")
}

type deniedBPFFile struct{}

func (deniedBPFFile) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
}

func TestBPFJITBytesFromObservations(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reading   *observation.Observation
		want      uint64
		wantError bool
	}{
		{name: "nil observation", wantError: true},
		{name: "wrong signal", reading: &observation.Observation{SignalId: memorysignals.TotalBytes.ID()}, wantError: true},
		{name: "missing outcome", reading: &observation.Observation{SignalId: memorysignals.BPFJITBytes.ID()}, wantError: true},
		{name: "no value", reading: &observation.Observation{
			SignalId: memorysignals.BPFJITBytes.ID(),
			Outcome:  &observation.Observation_NoValue{NoValue: &observation.NoValue{Reason: observation.NoValueReasonNotSupported}},
		}, wantError: true},
		{name: "wrong value type", reading: &observation.Observation{
			SignalId: memorysignals.BPFJITBytes.ID(),
			Outcome:  &observation.Observation_Value{Value: &observation.Value{Kind: &observation.Value_StringValue{StringValue: "0"}}},
		}, wantError: true},
		{name: "zero is a value", reading: &observation.Observation{
			SignalId: memorysignals.BPFJITBytes.ID(),
			Outcome:  &observation.Observation_Value{Value: &observation.Value{Kind: &observation.Value_UintValue{UintValue: 0}}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			size, err := bpfJITBytesFromObservations(&observation.ObservationBatch{Observations: []*observation.Observation{tc.reading}})
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.want, size)
		})
	}
}
