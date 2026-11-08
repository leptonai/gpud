package fabricmanager

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/nvidia/product"
)

func TestCheckBlackwellFabric(t *testing.T) {
	for _, name := range []string{"NVIDIA GB200", "NVIDIA GB300", "NVIDIA-B300-SXM6-AC", "NVIDIA B200", "NVIDIA B100"} {
		for _, healthy := range []bool{true, false} {
			t.Run(name+map[bool]string{true: "/healthy", false: "/unhealthy"}[healthy], func(t *testing.T) {
				rack := name == "NVIDIA GB200" || name == "NVIDIA GB300"
				collected, checkedFM := false, false
				comp := &component{
					ctx: context.Background(),
					nvmlInstance: &mockNVMLInstance{
						exists: true, productName: name,
						supportsFM:          product.SupportedFMByGPUProduct(name),
						supportsFabricState: product.SupportFabricStateByGPUProduct(name),
					},
					// A rack fabric is reachable even when only one local GPU is visible.
					getCountLspci: func(context.Context) (int, error) {
						if rack {
							return 1, nil
						}
						return 8, nil
					},
					checkNVSwitchExistsFunc: func() bool { return !rack },
					collectFabricStateFunc: func() fabricStateReport {
						collected = true
						if healthy {
							return fabricStateReport{Healthy: true}
						}
						return fabricStateReport{Reason: "bandwidth degraded"}
					},
					checkFMExistsFunc: func() bool { checkedFM = true; return true },
					checkFMActiveFunc: func() bool { return true },
				}
				cr := comp.Check().(*checkResult)
				require.True(t, collected, "fabric telemetry must be checked without a product-name override")
				assert.Equal(t, !rack, checkedFM, "only HGX/DGX systems require an on-node FM daemon")
				if healthy {
					assert.Equal(t, apiv1.HealthStateTypeHealthy, cr.health)
				} else {
					assert.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
					assert.Contains(t, cr.reason, "bandwidth degraded")
				}
			})
		}
	}
}
