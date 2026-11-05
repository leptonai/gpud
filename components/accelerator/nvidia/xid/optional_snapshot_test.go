package xid

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/bytedance/mockey"
	kmsgsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/kmsg"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/kmsg"
)

func TestSnapshotCollectorSelection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("snapshot collection is enabled on Linux")
	}
	t.Setenv("KMSG_FILE_PATH", "")
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			mockey.PatchConvey("snapshot uses the daemon's collector mode", t, func() {
				mockey.Mock(os.Geteuid).Return(0).Build()
				legacyErr, sensorErr := errors.New("legacy snapshot"), errors.New("sensor snapshot")
				legacy := mockey.Mock(kmsg.ReadAllWithOptions).Return(nil, legacyErr).Build()
				sensor := mockey.Mock(kmsgsensor.New).Return(nil, sensorErr).Build()
				comp, err := New(&components.GPUdInstance{RootCtx: t.Context(), ExperimentalHealthValidation: enabled})
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, comp.Close()) })
				_, err = comp.(*component).readAllKmsg(t.Context())
				if enabled {
					require.ErrorIs(t, err, sensorErr)
					require.Equal(t, 1, sensor.Times())
					require.Zero(t, legacy.Times())
				} else {
					require.ErrorIs(t, err, legacyErr)
					require.Equal(t, 1, legacy.Times())
					require.Zero(t, sensor.Times())
				}
			})
		})
	}
}
