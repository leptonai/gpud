// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package machineinfo

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	"github.com/urfave/cli"

	apiv1 "github.com/leptonai/gpud/api/v1"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	pkgfirmware "github.com/leptonai/gpud/pkg/firmware"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	"github.com/leptonai/gpud/pkg/netutil"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/providers"
	pkgprovidersall "github.com/leptonai/gpud/pkg/providers/all"
)

func TestCommand_DistinguishesAssignedPublicIPFromEgress(t *testing.T) {
	cases := []struct {
		name         string
		egress       string
		local        string
		metadata     string
		echoError    error
		wantPublicIP string
	}{
		{name: "NAT egress", egress: "1.2.3.4", local: "10.0.0.7"},
		{name: "local public assignment", egress: "1.2.3.4", local: "1.2.3.4", wantPublicIP: "1.2.3.4"},
		{name: "provider mapping differs from egress", egress: "1.2.3.4", local: "10.0.0.7", metadata: "5.6.7.8", wantPublicIP: "5.6.7.8"},
		{name: "echo failure with provider mapping", local: "10.0.0.7", metadata: "5.6.7.8", echoError: errors.New("echo unavailable"), wantPublicIP: "5.6.7.8"},
		{name: "echo failure without assignment", local: "10.0.0.7", echoError: errors.New("echo unavailable")},
	}
	for _, tc := range cases {
		for _, format := range []string{gpudcommon.OutputFormatPlain, gpudcommon.OutputFormatJSON} {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				mockey.PatchConvey(tc.name, t, func() {
					mockey.Mock(gpudcommon.StateFileFromContext).To(func(*cli.Context) (string, error) {
						return filepath.Join(t.TempDir(), "state.db"), nil
					}).Build()
					mockey.Mock(nvidianvml.New).To(func() (nvidianvml.Instance, error) {
						return nvidianvml.NewNoOp(), nil
					}).Build()
					mockey.Mock(pkgfirmware.Refresh).To(func(context.Context, pkgfirmware.Options) {}).Build()
					mockey.Mock(pkgmachineinfo.GetMachineInfo).To(func(nvidianvml.Instance) (*apiv1.MachineInfo, error) {
						return &apiv1.MachineInfo{}, nil
					}).Build()
					mockey.Mock(netutil.PublicIP).To(func() (string, error) {
						return tc.egress, tc.echoError
					}).Build()
					// Keep GetProvider real so both output formats exercise the assignment rule.
					mockey.Mock(pkgprovidersall.Detect).To(func(context.Context) (*providers.Info, error) {
						return &providers.Info{
							Provider: "test-provider", Region: "test-region",
							PublicIP: tc.metadata, PrivateIP: "10.0.0.7",
						}, nil
					}).Build()
					mockey.Mock(net.InterfaceAddrs).To(func() ([]net.Addr, error) {
						return []net.Addr{&net.IPNet{IP: net.ParseIP(tc.local), Mask: net.CIDRMask(32, 32)}}, nil
					}).Build()

					stdout, stderr := captureOutput(t, func() {
						require.NoError(t, Command(newCLIContext(t, []string{"--output-format", format})))
					})
					require.Empty(t, stderr)
					if format == gpudcommon.OutputFormatJSON {
						var out struct {
							EgressIP *string        `json:"egress_ip"`
							Provider providers.Info `json:"provider"`
						}
						require.NoError(t, json.Unmarshal([]byte(stdout), &out))
						require.NotNil(t, out.EgressIP)
						require.Equal(t, tc.egress, *out.EgressIP)
						require.Equal(t, tc.wantPublicIP, out.Provider.PublicIP)
						return
					}
					if tc.egress == "" {
						require.Contains(t, stdout, "Observed egress IP: unavailable")
					} else {
						require.Contains(t, stdout, "Observed egress IP: "+tc.egress)
					}
					if tc.wantPublicIP == "" {
						require.Contains(t, stdout, "Public IP assignment: unverified")
					} else {
						require.NotContains(t, stdout, "Public IP assignment: unverified")
						require.Contains(t, stdout, tc.wantPublicIP)
					}
					require.Contains(t, stdout, "Inbound reachability: not checked")
				})
			})
		}
	}
}
