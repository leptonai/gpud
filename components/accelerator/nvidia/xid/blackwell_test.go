package xid

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia/product"
	"github.com/leptonai/gpud/pkg/sqlite"
)

type productNVMLInstance struct {
	*mockNVMLInstance
	name string
}

func (m *productNVMLInstance) ProductName() string { return m.name }
func (m *productNVMLInstance) GetMemoryErrorManagementCapabilities() product.MemoryErrorManagementCapabilities {
	return product.SupportedMemoryMgmtCapsByGPUProduct(m.name)
}

func TestBlackwellRowRemappingOwnsXID(t *testing.T) {
	for _, name := range []string{"NVIDIA GB300", "NVIDIA-GB300", "NVIDIA B300 SXM6 AC", "NVIDIA-B300-SXM6-AC"} {
		t.Run(name, func(t *testing.T) {
			comp, err := New(&components.GPUdInstance{
				RootCtx:      context.Background(),
				NVMLInstance: &productNVMLInstance{mockNVMLInstance: createMockNVMLInstance(), name: name},
			})
			require.NoError(t, err)
			defer func() { assert.NoError(t, comp.Close()) }()
			c := mustComponent(t, comp)
			c.readAllKmsg = func(context.Context) ([]kmsg.Message, error) {
				return []kmsg.Message{
					{Timestamp: metav1.Now(), Message: "NVRM: Xid (PCI:0000:01:00): 63, Row remapping event"},
					{Timestamp: metav1.Now(), Message: "NVRM: Xid (PCI:0000:01:00): 64, Row remapping failure"},
				}, nil
			}
			result := c.Check()
			assert.Empty(t, mustCheckResult(t, result).FoundErrors)
			assert.Equal(t, apiv1.HealthStateTypeHealthy, result.HealthStateType())
			assert.Nil(t, mustCheckResult(t, result).suggestedActions)
		})
	}
}

func TestUpgradeRowRemappingEventOwnership(t *testing.T) {
	for _, name := range []string{"NVIDIA GB300", "NVIDIA B300 SXM6 AC", "NVIDIA V100"} {
		for _, legacy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/legacy=%t", name, legacy), func(t *testing.T) {
				ctx := context.Background()
				dbRW, dbRO, cleanup := sqlite.OpenTestDB(t)
				defer cleanup()
				store, err := eventstore.New(dbRW, dbRO, GetLookbackPeriod())
				require.NoError(t, err)
				bucket, err := store.Bucket(Name)
				require.NoError(t, err)
				defer bucket.Close()
				now := time.Now().UTC()
				c := &component{
					ctx: ctx, getTimeNowFunc: func() time.Time { return now },
					nvmlInstance: &productNVMLInstance{mockNVMLInstance: createMockNVMLInstance(), name: name},
					eventBucket:  bucket, rebootEventStore: pkghost.NewRebootEventStore(store),
				}
				// Old incorrectly classified events alternate with enough reboots to
				// reach hardware inspection unless ownership is reapplied on read.
				for i := 0; i < GetDefaultRebootThreshold()+2; i++ {
					ts := now.Add(time.Duration(-100+i*2) * time.Minute)
					code := uint64(63 + i%2)
					e := createXidEvent(ts, code, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem)
					if legacy {
						e.ExtraInfo[EventKeyErrorXidData] = fmt.Sprint(code)
					}
					require.NoError(t, bucket.Insert(ctx, e))
					require.NoError(t, bucket.Insert(ctx, eventstore.Event{Time: ts.Add(-time.Minute), Name: "reboot"}))
				}
				require.NoError(t, c.updateCurrentState())
				state := c.LastHealthStates()[0]
				if name == "NVIDIA V100" {
					assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
					require.NotNil(t, state.SuggestedActions)
				} else {
					assert.Equal(t, apiv1.HealthStateTypeHealthy, state.Health)
					assert.Nil(t, state.SuggestedActions)
				}
				history, err := c.Events(ctx, time.Time{})
				require.NoError(t, err)
				assert.Len(t, history, 2*(GetDefaultRebootThreshold()+2), "upgrade must preserve every diagnostic event")

				// A separate real fault must still trigger repair after the upgrade.
				require.NoError(t, bucket.Insert(ctx, createXidEvent(now, 79, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem)))
				require.NoError(t, c.updateCurrentState())
				state = c.LastHealthStates()[0]
				assert.Equal(t, apiv1.HealthStateTypeUnhealthy, state.Health)
				require.NotNil(t, state.SuggestedActions)
				assert.Contains(t, state.SuggestedActions.RepairActions, apiv1.RepairActionTypeRebootSystem)
			})
		}
	}
}
