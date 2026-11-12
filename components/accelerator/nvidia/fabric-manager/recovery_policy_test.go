package fabricmanager

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components/accelerator/nvidia/fabric"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	"github.com/leptonai/gpud/pkg/nvidia/product"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func TestFabricUnavailableAcrossBootsDoesNotRequestReboot(t *testing.T) {
	for _, name := range []string{"NVIDIA H200", "NVIDIA B300 SXM6 AC", "NVIDIA GB200 NVL72", "NVIDIA GB300"} {
		t.Run(name, func(t *testing.T) {
			db, _, cleanup := sqlite.OpenTestDB(t)
			defer cleanup()
			now := time.Now()
			report := fabricStateReport{Pending: true}
			for boot := range 3 {
				c := &component{
					ctx: context.Background(),
					nvmlInstance: &mockNVMLInstance{exists: true, productName: name,
						supportsFM: product.SupportedFMByGPUProduct(name), supportsFabricState: true},
					getTimeNowFunc:          func() time.Time { return now },
					collectFabricStateFunc:  func() fabricStateReport { return report },
					checkNVSwitchExistsFunc: func() bool { return true },
					checkFMExistsFunc: func() bool {
						require.True(t, product.SupportedFMByGPUProduct(name), "rack fabrics must not probe local FM")
						return true
					},
					checkFMActiveFunc: func() bool { return false },
				}
				require.NoError(t, c.fabricRegistration.Open(c.ctx, db, Name, fmt.Sprint(boot)))
				report = fabricStateReport{Pending: true}
				require.Equal(t, apiv1.HealthStateTypeDegraded, c.Check().HealthStateType())
				for _, elapsed := range []time.Duration{fabric.RegistrationTimeout, 24 * time.Hour} {
					now = now.Add(elapsed)
					cr := c.Check().(*checkResult)
					require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
					require.Contains(t, cr.reason, "did not complete")
					require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.HealthStates()[0].SuggestedActions.RepairActions)
					require.Contains(t, cr.suggestedActions.Description, "Inspect fabric")
				}
				// An explicit fault does not prove that rebooting this host can repair it.
				report = fabricStateReport{Failed: true, Reason: "fabric unhealthy"}
				cr := c.Check().(*checkResult)
				require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
				require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.suggestedActions.RepairActions)
			}
		})
	}
}

func TestPendingRegistrationChecksLocalManager(t *testing.T) {
	for _, tc := range []struct {
		name           string
		exists, active bool
		health         apiv1.HealthStateType
		reason         string
	}{
		{"missing executable", false, false, apiv1.HealthStateTypeUnhealthy, "executable not found"},
		{"inactive", true, false, apiv1.HealthStateTypeDegraded, "found but not active"},
		{"active but pending", true, true, apiv1.HealthStateTypeDegraded, "found and active"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			activeChecks := 0
			c := &component{
				ctx:                    context.Background(),
				nvmlInstance:           &mockNVMLInstance{exists: true, productName: "NVIDIA H200", supportsFM: true, supportsFabricState: true},
				collectFabricStateFunc: func() fabricStateReport { return fabricStateReport{Pending: true} },
				checkFMExistsFunc:      func() bool { return tc.exists },
				checkFMActiveFunc:      func() bool { activeChecks++; return tc.active },
			}
			cr := c.Check().(*checkResult)
			require.Equal(t, tc.health, cr.health)
			require.Contains(t, cr.reason, tc.reason)
			require.Equal(t, tc.active, cr.FabricManagerActive)
			require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeIgnoreNoActionRequired}, cr.suggestedActions.RepairActions)
			if tc.exists {
				require.Equal(t, 1, activeChecks)
			} else {
				require.Zero(t, activeChecks)
			}
			// Independent device faults must survive the local manager diagnosis.
			for _, err := range []error{nvmlerrors.ErrGPULost, nvmlerrors.ErrGPURequiresReset} {
				c.collectFabricStateFunc = func() fabricStateReport { return fabricStateReport{Pending: true, Failed: true, Err: err} }
				cr = c.Check().(*checkResult)
				require.Equal(t, apiv1.HealthStateTypeUnhealthy, cr.health)
				require.Equal(t, []apiv1.RepairActionType{apiv1.RepairActionTypeRebootSystem}, cr.suggestedActions.RepairActions)
			}
		})
	}
}
