package components_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	kmsgsource "github.com/dsx-ai-factory/health-validation/collect/source/kmsg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/nfs"
)

func TestKernelSubscriptionsShareOneSourceAndFilterInComponents(t *testing.T) {
	mockey.PatchConvey("one daemon stream with independent component interpretation", t, func() {
		var mu sync.Mutex
		var opens, closes atomic.Int32
		at := time.Unix(1700000000, 0)
		records := []kmsgsource.Message{
			{BootID: "boot", Sequence: 1, Timestamp: at, Priority: 6, Body: "INFO: task worker:1177 blocked for more than 120 seconds."},
			{BootID: "boot", Sequence: 2, Timestamp: at, Priority: 6, Body: "nfs: server 192.0.2.1 not responding, timed out"},
			{BootID: "boot", Sequence: 3, Timestamp: at, Priority: 6, Body: "unrelated kernel message"},
		}
		mockey.Mock(kmsgsource.NewStream).To(func(int) (*kmsgsource.Stream, error) {
			opens.Add(1)
			return &kmsgsource.Stream{}, nil
		}).Build()
		mockey.Mock((*kmsgsource.Stream).Read).To(func(_ *kmsgsource.Stream, _ context.Context) ([]kmsgsource.Message, error) {
			mu.Lock()
			defer mu.Unlock()
			return append([]kmsgsource.Message(nil), records...), nil
		}).Build()
		mockey.Mock((*kmsgsource.Stream).Close).To(func(*kmsgsource.Stream) error {
			closes.Add(1)
			return nil
		}).Build()

		g := &components.GPUdInstance{RootCtx: t.Context()}
		consumer := func(match func(string) (string, string), events chan<- string) func(context.Context, *observation.ObservationBatch) {
			return func(ctx context.Context, batch *observation.ObservationBatch) {
				messages, err := components.KernelMessages(batch)
				assert.NoError(t, err)
				for _, message := range messages {
					if name, _ := match(message.Message); name != "" {
						select {
						case events <- name:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}
		cpuEvents, nfsEvents := make(chan string, 4), make(chan string, 4)
		stopCPU, err := g.ObserveKernel(t.Context(), consumer(cpu.Match, cpuEvents))
		require.NoError(t, err)
		defer stopCPU()
		stopNFS, err := g.ObserveKernel(t.Context(), consumer(nfs.Match, nfsEvents))
		require.NoError(t, err)
		defer stopNFS()
		require.Eventually(t, func() bool { return len(cpuEvents) == 1 && len(nfsEvents) == 1 }, 5*time.Second, time.Millisecond)
		assert.Equal(t, "cpu_blocked_too_long", <-cpuEvents)
		assert.Equal(t, "nfs_server_not_responding", <-nfsEvents)
		assert.EqualValues(t, 1, opens.Load())

		stopCPU()
		assert.Zero(t, closes.Load())
		mu.Lock()
		records = append(records, kmsgsource.Message{BootID: "boot", Sequence: 4, Timestamp: at, Priority: 6, Body: "nfs: server 192.0.2.1 OK"})
		mu.Unlock()
		require.Eventually(t, func() bool { return len(nfsEvents) == 1 }, 5*time.Second, time.Millisecond)
		assert.Equal(t, "nfs_server_ok", <-nfsEvents)
		assert.Empty(t, cpuEvents)
		assert.EqualValues(t, 1, opens.Load())
		stopNFS()
		assert.EqualValues(t, 1, closes.Load())
	})
}
