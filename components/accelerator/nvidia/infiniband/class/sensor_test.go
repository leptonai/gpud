package class

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/dsx-ai-factory/health-validation/collect/observation"
	ibsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/infiniband"
	ibsignals "github.com/dsx-ai-factory/health-validation/collect/signals/infiniband"
	ibsource "github.com/dsx-ai-factory/health-validation/collect/source/infiniband"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaderFixtureCompatibility(t *testing.T) {
	devices, err := New("testdata/sys-class-infiniband-h100.0").Read(t.Context())
	require.NoError(t, err)
	// Captured from GPUd's old collector before replacing its sysfs parser.
	expected, err := os.ReadFile("testdata/devices-h100.json")
	require.NoError(t, err)
	var previous Devices
	require.NoError(t, json.Unmarshal(expected, &previous))
	for i := range previous {
		ports := make([]Port, 0, len(previous[i].Ports))
		for _, port := range previous[i].Ports {
			if strings.EqualFold(port.LinkLayer, "InfiniBand") {
				ports = append(ports, port)
			}
		}
		previous[i].Ports = ports
	}
	expected, err = json.Marshal(previous)
	require.NoError(t, err)
	actual, err := json.Marshal(devices)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(actual))
}

func portFiles() fstest.MapFS {
	return fstest.MapFS{
		"mlx5_0/fw_ver":             {Data: []byte("28.41.1000\n")},
		"mlx5_0/ports/1/link_layer": {Data: []byte("InfiniBand\n")},
		"mlx5_0/ports/1/state":      {Data: []byte("4: ACTIVE\n")},
		"mlx5_0/ports/1/phys_state": {Data: []byte("5: LinkUp\n")},
		"mlx5_0/ports/1/rate":       {Data: []byte("400 Gb/sec (4X NDR)\n")},
	}
}

func readerFor(files fs.FS, excluded ...string) *Reader {
	return &Reader{sensor: ibsensor.New(ibsource.New(files, excluded...))}
}

func TestReaderOptionalCounters(t *testing.T) {
	files := portFiles()
	files["mlx5_0/ports/1/counters/link_downed"] = &fstest.MapFile{Data: []byte("0")}
	files["mlx5_0/ports/1/counters/port_rcv_data"] = &fstest.MapFile{Data: []byte("23")}
	files["mlx5_0/ports/1/counters/port_xmit_data"] = &fstest.MapFile{Data: []byte("42")}
	files["mlx5_0/ports/1/counters/port_rcv_errors"] = &fstest.MapFile{Data: []byte("N/A (no PMA)")}
	files["mlx5_0/ports/1/hw_counters/rx_write_requests"] = &fstest.MapFile{Data: []byte("123")}
	devices, err := readerFor(files).Read(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 1)
	require.Len(t, devices[0].Ports, 1)
	port := devices[0].Ports[0]
	assert.Equal(t, uint64(0), *port.Counters.LinkDowned)
	assert.Equal(t, uint64(92), *port.Counters.PortRcvData)
	assert.Equal(t, uint64(168), *port.Counters.PortXmitData)
	assert.Equal(t, uint64(123), *port.HWCounters.RxWriteRequests)
	assert.Nil(t, port.Counters.LinkErrorRecovery)
	assert.Nil(t, port.Counters.PortRcvErrors)
	assert.Empty(t, devices[0].BoardID)
	assert.Empty(t, devices[0].HCAType)
}

func TestReaderEnumeration(t *testing.T) {
	files := portFiles()
	for name, file := range portFiles() {
		files[strings.Replace(name, "mlx5_0", "mlx5_2", 1)] = file
		files[strings.Replace(name, "mlx5_0", "mlx5_10", 1)] = file
		files[strings.Replace(name, "mlx5_0", "other", 1)] = file
		if strings.Contains(name, "/ports/1/") {
			files[strings.Replace(name, "/ports/1/", "/ports/10/", 1)] = file
			files[strings.Replace(name, "/ports/1/", "/ports/2/", 1)] = file
		}
	}
	files["mlx5_0/ports/invalid"] = &fstest.MapFile{Mode: fs.ModeDir}
	// The public sensor omits Ethernet port observations.
	files["mlx5_0/ports/2/link_layer"] = &fstest.MapFile{Data: []byte("Ethernet")}
	devices, err := readerFor(files, "mlx5_2").Read(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.Equal(t, "mlx5_0", devices[0].Name)
	assert.Equal(t, "mlx5_10", devices[1].Name)
	require.Len(t, devices[0].Ports, 2)
	assert.Equal(t, []uint{1, 10}, []uint{devices[0].Ports[0].Port, devices[0].Ports[1].Port})

	devices, err = readerFor(files, "mlx5_0", "mlx5_2", "mlx5_10").Read(t.Context())
	require.NoError(t, err)
	assert.Equal(t, Devices{}, devices)

	files = fstest.MapFS{
		"mlx5_0/fw_ver": {Data: []byte("28.41.1000")},
		"mlx5_0/ports":  {Mode: fs.ModeDir},
	}
	devices, err = readerFor(files).Read(t.Context())
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, []Port{}, devices[0].Ports)
}

type failingFS struct {
	fs.FS
	failures map[string]error
	opens    map[string]int
}

func (f *failingFS) Open(name string) (fs.File, error) {
	f.opens[name]++
	if err := f.failures[name]; err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	return f.FS.Open(name)
}

func TestReaderFailurePolicy(t *testing.T) {
	const counter = "mlx5_0/ports/1/counters/link_downed"
	for _, tc := range []struct {
		name  string
		path  string
		err   error
		fatal bool
	}{
		{"missing root", ".", fs.ErrNotExist, true},
		{"missing firmware", "mlx5_0/fw_ver", fs.ErrNotExist, true},
		{"missing ports", "mlx5_0/ports", fs.ErrNotExist, true},
		{"missing link layer", "mlx5_0/ports/1/link_layer", fs.ErrNotExist, true},
		{"missing state", "mlx5_0/ports/1/state", fs.ErrNotExist, true},
		{"missing physical state", "mlx5_0/ports/1/phys_state", fs.ErrNotExist, true},
		{"missing rate", "mlx5_0/ports/1/rate", fs.ErrNotExist, true},
		{"missing board", "mlx5_0/board_id", fs.ErrNotExist, false},
		{"denied board", "mlx5_0/board_id", fs.ErrPermission, true},
		{"denied hca type", "mlx5_0/hca_type", fs.ErrPermission, true},
		{"missing counters", "mlx5_0/ports/1/counters", fs.ErrNotExist, false},
		{"denied counters", "mlx5_0/ports/1/counters", fs.ErrPermission, false},
		{"denied hw counters", "mlx5_0/ports/1/hw_counters", fs.ErrPermission, false},
		{"missing counter", counter, fs.ErrNotExist, false},
		{"denied counter", counter, fs.ErrPermission, false},
		{"restricted counter", counter, syscall.EINVAL, false},
		{"unsupported counter", counter, syscall.EOPNOTSUPP, false},
		{"broken counter", counter, errors.New("I/O error"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := portFiles()
			files[counter] = &fstest.MapFile{Data: []byte("0")}
			failing := &failingFS{FS: files, failures: map[string]error{tc.path: tc.err}, opens: make(map[string]int)}
			devices, err := readerFor(failing).Read(t.Context())
			if tc.fatal {
				require.Error(t, err)
				assert.Nil(t, devices, "required failures must not return partial health data")
			} else {
				require.NoError(t, err)
				if tc.name == "missing board" || tc.name == "denied hw counters" {
					assert.Equal(t, uint64(0), *devices[0].Ports[0].Counters.LinkDowned)
				} else {
					assert.Nil(t, devices[0].Ports[0].Counters.LinkDowned)
				}
			}
		})
	}
}

func TestReaderRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct{ path, value string }{
		{"state", "unknown"},
		{"phys_state", "-1: LinkUp"},
		{"rate", "NaN Gb/sec"},
		{"counters/link_downed", "-1"},
		{"counters/link_downed", "9223372036854775808"},
		{"counters/port_rcv_data", "2305843009213693952"},
	} {
		t.Run(tc.path+"/"+tc.value, func(t *testing.T) {
			files := portFiles()
			files["mlx5_0/ports/1/"+tc.path] = &fstest.MapFile{Data: []byte(tc.value)}
			devices, err := readerFor(files).Read(t.Context())
			require.Error(t, err)
			assert.Nil(t, devices)
		})
	}
}

func TestReaderRestrictedCounterCache(t *testing.T) {
	for _, group := range []string{"counters/link_downed", "hw_counters/rx_write_requests"} {
		t.Run(group, func(t *testing.T) {
			name := "mlx5_0/ports/1/" + group
			files := portFiles()
			files[name] = &fstest.MapFile{Data: []byte("0")}
			failing := &failingFS{FS: files, failures: map[string]error{name: syscall.EINVAL}, opens: make(map[string]int)}
			reader := readerFor(failing)
			_, err := reader.Read(t.Context())
			require.NoError(t, err)
			delete(failing.failures, name)
			// Port state is refreshed while the restricted counter remains cached.
			files["mlx5_0/ports/1/state"].Data = []byte("1: DOWN")
			devices, err := reader.Read(t.Context())
			require.NoError(t, err)
			assert.Equal(t, "DOWN", devices[0].Ports[0].State)
			assert.Nil(t, devices[0].Ports[0].Counters.LinkDowned)
			assert.Nil(t, devices[0].Ports[0].HWCounters.RxWriteRequests)
			assert.Equal(t, 1, failing.opens[name])
			_, err = readerFor(failing).Read(t.Context())
			require.NoError(t, err)
			assert.Equal(t, 2, failing.opens[name], "a new source retries restricted counters")
		})
	}
}

func TestReaderCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	devices, err := readerFor(portFiles()).Read(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, devices)
}

func TestDevicesFromObservations(t *testing.T) {
	port := observation.InfiniBandPortTarget("mlx5_0", 1)
	for _, tc := range []struct {
		name      string
		batch     *observation.ObservationBatch
		wantError bool
	}{
		{name: "nil batch", wantError: true},
		{name: "empty inventory", batch: &observation.ObservationBatch{}},
		{name: "nil observation", batch: &observation.ObservationBatch{Observations: []*observation.Observation{nil}}, wantError: true},
		{name: "missing outcome", batch: &observation.ObservationBatch{Observations: []*observation.Observation{{SignalId: ibsignals.State.ID(), Target: port}}}, wantError: true},
		{name: "required value unavailable", batch: &observation.ObservationBatch{Observations: []*observation.Observation{{
			SignalId: ibsignals.State.ID(), Target: port,
			Outcome: &observation.Observation_NoValue{NoValue: &observation.NoValue{Reason: observation.NoValueReasonNotSupported}},
		}}}, wantError: true},
		{name: "optional counter unsupported", batch: &observation.ObservationBatch{Observations: []*observation.Observation{{
			SignalId: ibsignals.LinkDowned.ID(), Target: port,
			Outcome: &observation.Observation_NoValue{NoValue: &observation.NoValue{Reason: observation.NoValueReasonNotSupported}},
		}}}},
		{name: "whole collection unavailable", batch: &observation.ObservationBatch{Observations: []*observation.Observation{{
			SignalId: ibsignals.BoardID.ID(),
			Outcome:  &observation.Observation_CollectionError{CollectionError: &observation.CollectionError{Category: observation.CollectionErrorCategoryUnavailable, Operation: "read"}},
		}}}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			devices, err := devicesFromObservations(tc.batch)
			if tc.wantError {
				require.Error(t, err)
				assert.Nil(t, devices)
			} else {
				require.NoError(t, err)
				assert.Equal(t, Devices{}, devices)
			}
		})
	}
}
