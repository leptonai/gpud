package memory

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"runtime"

	"github.com/dsx-ai-factory/health-validation/collect"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	memorysensor "github.com/dsx-ai-factory/health-validation/collect/sensors/memory"
	memorysignals "github.com/dsx-ai-factory/health-validation/collect/signals/memory"
)

func isLinux() bool {
	return runtime.GOOS == "linux"
}

// getCurrentBPFJITBufferBytes returns the current BPF JIT buffer size in bytes.
// Useful to debug "failed to create shim task: OCI" due to insufficient BPF JIT buffer.
// ref. https://github.com/awslabs/amazon-eks-ami/issues/1179
// ref. https://github.com/deckhouse/deckhouse/issues/7402
func getCurrentBPFJITBufferBytes() (uint64, error) {
	if !isLinux() {
		return 0, nil
	}

	b, err := readBPFJITBufferBytes(nil)

	// if not root, this can fail
	// e.g.,
	// "open /proc/vmallocinfo: permission denied"
	if err != nil && os.Geteuid() != 0 {
		return 0, nil
	}

	return b, err
}

// readBPFJITBufferBytes selects only BPF accounting; it does not read /proc/meminfo.
// A nil filesystem reads the host's /proc/vmallocinfo.
func readBPFJITBufferBytes(files fs.FS) (uint64, error) {
	batch, err := memorysensor.New(files).Measure(context.Background(), collect.WithSignalID(memorysignals.BPFJITBytes.ID()))
	if err != nil {
		return 0, err
	}
	return bpfJITBytesFromObservations(batch)
}

// bpfJITBytesFromObservations is the memory component's shim from sensor output
// to GPUd's byte count. It performs no collection or health evaluation.
func bpfJITBytesFromObservations(batch *observation.ObservationBatch) (uint64, error) {
	if len(batch.GetObservations()) != 1 {
		return 0, fmt.Errorf("BPF JIT sensor returned %d observations, expected one", len(batch.GetObservations()))
	}
	reading := batch.Observations[0]
	if reading.GetSignalId() != memorysignals.BPFJITBytes.ID() {
		return 0, fmt.Errorf("BPF JIT sensor returned unexpected signal %q", reading.GetSignalId())
	}
	if failure := reading.GetCollectionError(); failure != nil {
		return 0, fmt.Errorf("BPF JIT collection %s: %s", failure.Category, failure.Detail)
	}
	value, ok := reading.GetValue().GetKind().(*observation.Value_UintValue)
	if !ok {
		return 0, fmt.Errorf("BPF JIT sensor returned no unsigned byte count")
	}
	return value.UintValue, nil
}
