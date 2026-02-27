package edge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/control/controlknobs"
	"tailscale.com/net/netcheck"
	"tailscale.com/net/netmon"
	"tailscale.com/net/portmapper"
	"tailscale.com/tailcfg"
	"tailscale.com/types/logger"

	"github.com/leptonai/gpud/pkg/netutil/latency"
	"github.com/leptonai/gpud/pkg/netutil/latency/edge/derpmap"
)

func TestMeasure_UsesDefaultDERPMapWithMockey(t *testing.T) {
	mockey.PatchConvey("Measure uses default derp map", t, func() {
		called := false
		mockey.Mock(measureDERP).To(func(ctx context.Context, m *tailcfg.DERPMap, opts ...OpOption) (latency.Latencies, error) {
			called = true
			assert.Equal(t, &derpmap.DefaultDERPMap, m)
			op := &Op{}
			_ = op.applyOpts(opts)
			assert.True(t, op.verbose)
			return latency.Latencies{{Provider: "test-provider"}}, nil
		}).Build()

		latencies, err := Measure(context.Background(), WithVerbose(true))
		require.NoError(t, err)
		require.Len(t, latencies, 1)
		assert.True(t, called)
	})
}

func TestMeasureDERP_NetmonErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("measureDERP returns netmon error", t, func() {
		mockey.Mock(netmon.New).To(func(logf logger.Logf) (*netmon.Monitor, error) {
			return nil, errors.New("netmon failed")
		}).Build()

		_, err := measureDERP(context.Background(), &tailcfg.DERPMap{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "netmon failed")
	})
}

func TestMeasureDERP_SuccessWithMockey(t *testing.T) {
	mockey.PatchConvey("measureDERP returns sorted latencies", t, func() {
		realMon, err := netmon.New(logger.Discard)
		require.NoError(t, err)

		mockey.Mock(netmon.New).To(func(logf logger.Logf) (*netmon.Monitor, error) {
			return realMon, nil
		}).Build()
		mockey.Mock(portmapper.NewClient).To(func(logf logger.Logf, netMon *netmon.Monitor, debug *portmapper.DebugKnobs, control *controlknobs.Knobs, onChange func()) *portmapper.Client {
			return &portmapper.Client{}
		}).Build()
		mockey.Mock((*netcheck.Client).GetReport).To(func(c *netcheck.Client, _ context.Context, m *tailcfg.DERPMap, _ *netcheck.GetReportOpts) (*netcheck.Report, error) {
			assert.True(t, c.Verbose)
			return &netcheck.Report{
				RegionLatency: map[int]time.Duration{
					1: 200 * time.Millisecond,
					2: 100 * time.Millisecond,
				},
			}, nil
		}).Build()

		derp := &tailcfg.DERPMap{
			Regions: map[int]*tailcfg.DERPRegion{
				1: {RegionID: 1, RegionName: "Tokyo"},
				2: {RegionID: 2, RegionName: "Seattle"},
			},
		}

		latencies, err := measureDERP(context.Background(), derp, WithVerbose(true))
		require.NoError(t, err)
		require.Len(t, latencies, 2)
		assert.Equal(t, "Seattle", latencies[0].RegionName)
		assert.Equal(t, "us-west-2", latencies[0].RegionCode)
		assert.Equal(t, ProviderTailscaleDERP, latencies[0].Provider)
	})
}

func TestMeasureDERP_RegionNotFoundWithMockey(t *testing.T) {
	mockey.PatchConvey("measureDERP fails when report has unknown region id", t, func() {
		realMon, err := netmon.New(logger.Discard)
		require.NoError(t, err)

		mockey.Mock(netmon.New).To(func(logf logger.Logf) (*netmon.Monitor, error) {
			return realMon, nil
		}).Build()
		mockey.Mock(portmapper.NewClient).To(func(logf logger.Logf, netMon *netmon.Monitor, debug *portmapper.DebugKnobs, control *controlknobs.Knobs, onChange func()) *portmapper.Client {
			return &portmapper.Client{}
		}).Build()
		mockey.Mock((*netcheck.Client).GetReport).To(func(c *netcheck.Client, _ context.Context, m *tailcfg.DERPMap, _ *netcheck.GetReportOpts) (*netcheck.Report, error) {
			return &netcheck.Report{
				RegionLatency: map[int]time.Duration{
					999: 100 * time.Millisecond,
				},
			}, nil
		}).Build()

		derp := &tailcfg.DERPMap{
			Regions: map[int]*tailcfg.DERPRegion{
				1: {RegionID: 1, RegionName: "Tokyo"},
			},
		}

		latencies, err := measureDERP(context.Background(), derp)
		require.Error(t, err)
		assert.Nil(t, latencies)
		assert.Contains(t, err.Error(), "region 999 not found in derpmap")
	})
}

func TestMeasureDERP_RegionCodeNotFoundWithMockey(t *testing.T) {
	mockey.PatchConvey("measureDERP fails when region name has no aws mapping", t, func() {
		realMon, err := netmon.New(logger.Discard)
		require.NoError(t, err)

		mockey.Mock(netmon.New).To(func(logf logger.Logf) (*netmon.Monitor, error) {
			return realMon, nil
		}).Build()
		mockey.Mock(portmapper.NewClient).To(func(logf logger.Logf, netMon *netmon.Monitor, debug *portmapper.DebugKnobs, control *controlknobs.Knobs, onChange func()) *portmapper.Client {
			return &portmapper.Client{}
		}).Build()
		mockey.Mock((*netcheck.Client).GetReport).To(func(c *netcheck.Client, _ context.Context, m *tailcfg.DERPMap, _ *netcheck.GetReportOpts) (*netcheck.Report, error) {
			return &netcheck.Report{
				RegionLatency: map[int]time.Duration{
					1: 100 * time.Millisecond,
				},
			}, nil
		}).Build()

		derp := &tailcfg.DERPMap{
			Regions: map[int]*tailcfg.DERPRegion{
				1: {RegionID: 1, RegionName: "Unknown Region"},
			},
		}

		latencies, err := measureDERP(context.Background(), derp)
		require.Error(t, err)
		assert.Nil(t, latencies)
		assert.Contains(t, err.Error(), "failed to get AWS region for Unknown Region")
	})
}
