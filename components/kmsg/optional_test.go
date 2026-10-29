package kmsg_test

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	"github.com/leptonai/gpud/components/accelerator/nvidia/nccl"
	"github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	"github.com/leptonai/gpud/components/accelerator/nvidia/peermem"
	"github.com/leptonai/gpud/components/accelerator/nvidia/sxid"
	"github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	"github.com/leptonai/gpud/components/cpu"
	"github.com/leptonai/gpud/components/disk"
	componentkmsg "github.com/leptonai/gpud/components/kmsg"
	"github.com/leptonai/gpud/components/memory"
	"github.com/leptonai/gpud/components/nfs"
	componentos "github.com/leptonai/gpud/components/os"
	"github.com/leptonai/gpud/pkg/eventstore"
	legacy "github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestComponentsSelectKernelCollector(t *testing.T) {
	constructors := map[string]components.InitFunc{
		"cpu": cpu.New, "disk": disk.New, "memory": memory.New, "nfs": nfs.New, "os": componentos.New,
		"infiniband": infiniband.New, "nccl": nccl.New, "nvlink": nvlink.New, "peermem": peermem.New, "sxid": sxid.New, "xid": xid.New,
	}
	dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
	defer cleanup()
	store, err := eventstore.New(dbRW, dbRO, eventstore.DefaultRetention)
	require.NoError(t, err)
	for name, newComponent := range constructors {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", name, enabled), func(t *testing.T) {
				mockey.PatchConvey("only the selected source is opened", t, func() {
					mockey.Mock(os.Geteuid).Return(0).Build()
					legacyErr, sensorErr := errors.New("legacy collector"), errors.New("sensor collector")
					syncer := mockey.Mock(legacy.NewSyncer).Return(nil, legacyErr).Build()
					watcher := mockey.Mock(legacy.NewWatcher).Return(nil, legacyErr).Build()
					sensor := mockey.Mock((*componentkmsg.Stream).Subscribe).Return(nil, sensorErr).Build()
					c, err := newComponent(&components.GPUdInstance{RootCtx: t.Context(), EventStore: store, ExperimentalHealthValidation: enabled})
					if name == "nfs" && runtime.GOOS != "linux" {
						require.NoError(t, err)
						require.NoError(t, c.Close())
						require.Zero(t, syncer.Times()+watcher.Times()+sensor.Times())
						return
					}
					if enabled {
						require.ErrorIs(t, err, sensorErr)
						require.Equal(t, 1, sensor.Times())
						require.Zero(t, syncer.Times()+watcher.Times())
					} else {
						require.ErrorIs(t, err, legacyErr)
						require.Equal(t, 1, syncer.Times()+watcher.Times())
						require.Zero(t, sensor.Times())
					}
				})
			})
		}
	}
}
