package session

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	configcommon "github.com/leptonai/gpud/pkg/config/common"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/providers"
	providersall "github.com/leptonai/gpud/pkg/providers/all"
)

func TestSessionGossipIncludesRefreshedInstanceType(t *testing.T) {
	mockey.PatchConvey("session gossip carries cached metadata and preserves collection errors", t, func() {
		failure := errors.New("machine info unavailable")
		fail := false
		mockey.Mock(pkgmachineinfo.CreateGossipRequestWithContainerd).To(func(id string, _ nvidianvml.Instance, _ configcommon.ContainerdConfig) (*apiv1.GossipRequest, error) {
			if fail {
				return nil, failure
			}
			return &apiv1.GossipRequest{MachineID: id}, nil
		}).Build()
		synctest.Test(t, func(t *testing.T) {
			original := providersall.All
			defer func() { providersall.All = original }()
			providersall.All = []providers.Detector{providers.NewIMDSWithRegion("aws",
				func(context.Context) (string, error) { return "aws", nil }, nil, nil, nil, nil, nil,
				providers.WithInstanceType(func(context.Context) (string, error) { return "p5.48xlarge", nil }))}
			s, err := NewSession(t.Context(), "", "", "", WithDataDir(t.TempDir()), WithPipeInterval(time.Second), WithComponentsRegistry(components.NewRegistry(nil)))
			require.NoError(t, err)
			defer func() { s.Stop(); synctest.Wait() }()
			synctest.Wait()
			req, err := s.createGossipRequestFunc("machine", nil)
			require.NoError(t, err)
			assert.Equal(t, "machine", req.MachineID)
			assert.Equal(t, "p5.48xlarge", req.ProviderInstanceType)
			fail = true
			req, err = s.createGossipRequestFunc("machine", nil)
			require.ErrorIs(t, err, failure)
			assert.Nil(t, req)
		})
	})
}
