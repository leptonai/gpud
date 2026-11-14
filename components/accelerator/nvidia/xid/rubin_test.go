package xid

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/nvidia/product"
	"github.com/leptonai/gpud/pkg/nvsentinel"
)

type rubinXidInstance struct{ *productNVMLInstance }

func (*rubinXidInstance) Architecture() string { return "rubin" }
func (m *rubinXidInstance) GetMemoryErrorManagementCapabilities() product.MemoryErrorManagementCapabilities {
	return product.CapabilitiesForGPU(m.ProductName(), m.Architecture(), product.HostLinkUnknown).MemoryManagement
}

func TestRubinMemoryFailureIsNotDiscarded(t *testing.T) {
	inst := &rubinXidInstance{&productNVMLInstance{mockNVMLInstance: createMockNVMLInstance(), name: "NVIDIA-Graphics-Device"}}
	comp, err := New(&components.GPUdInstance{RootCtx: context.Background(), NVMLInstance: inst})
	require.NoError(t, err)
	defer func() { require.NoError(t, comp.Close()) }()
	c := mustComponent(t, comp)
	c.readAllKmsg = func(context.Context) ([]kmsg.Message, error) {
		return []kmsg.Message{
			{Timestamp: metav1.Now(), Message: "NVRM: Xid (PCI:0000:01:00): 63, Row remapping event"},
			{Timestamp: metav1.Now(), Message: "NVRM: Xid (PCI:0000:01:00): 64, GPU memory remapping failure"},
		}, nil
	}
	cr := mustCheckResult(t, c.Check())
	require.Len(t, cr.FoundErrors, 1)
	require.Equal(t, 64, cr.FoundErrors[0].Xid)
	for _, legacy := range []bool{false, true} {
		events := eventstore.Events{
			createXidEvent(time.Now(), 63, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
			createXidEvent(time.Now(), 64, apiv1.EventTypeFatal, apiv1.RepairActionTypeRebootSystem),
		}
		if legacy {
			events[0].ExtraInfo[EventKeyErrorXidData] = "63"
			events[1].ExtraInfo[EventKeyErrorXidData] = "64"
		}
		require.Equal(t, events[1:], c.eventsForHealth(events))
	}
	code, _, matched := c.matchNVSentinelXid(nvsentinel.HealthEvent{ComponentClass: "GPU", ErrorCodes: []string{"64"}})
	require.True(t, matched)
	require.Equal(t, 64, code)
	_, _, matched = c.matchNVSentinelXid(nvsentinel.HealthEvent{ComponentClass: "GPU", ErrorCodes: []string{"63"}})
	require.False(t, matched)
}
