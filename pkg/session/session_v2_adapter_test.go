// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
	sessionv2 "github.com/leptonai/gpud/pkg/session/v2"
)

func TestRequestFromV2ConvertsConcreteCommands(t *testing.T) {
	start := time.Unix(100, 200).UTC()
	end := time.Unix(300, 400).UTC()
	regex := "^ok$"
	tests := []struct {
		name    string
		request *sessionv2.Request
		want    Request
	}{
		{
			name:    "states",
			request: newV2Request(&sessionv2.Request_GetHealthStates{GetHealthStates: &sessionv2.GetHealthStatesCommand{}}),
			want:    Request{Method: "states"},
		},
		{
			name: "events",
			request: newV2Request(&sessionv2.Request_GetEvents{GetEvents: &sessionv2.GetEventsCommand{
				StartTime: timestamppb.New(start),
				EndTime:   timestamppb.New(end),
			}}),
			want: Request{Method: "events", StartTime: start, EndTime: end},
		},
		{
			name: "zero event times",
			request: newV2Request(&sessionv2.Request_GetEvents{GetEvents: &sessionv2.GetEventsCommand{
				StartTime: timestamppb.New(time.Time{}),
				EndTime:   timestamppb.New(time.Time{}),
			}}),
			want: Request{Method: "events"},
		},
		{
			name:    "metrics",
			request: newV2Request(&sessionv2.Request_GetMetrics{GetMetrics: &sessionv2.GetMetricsCommand{SinceNanos: int64(3 * time.Minute)}}),
			want:    Request{Method: "metrics", Since: 3 * time.Minute},
		},
		{
			name: "update",
			request: newV2Request(&sessionv2.Request_Update{Update: &sessionv2.UpdateCommand{
				Version:    "v1.2.3",
				SinceNanos: int64(4 * time.Second),
			}}),
			want: Request{Method: "update", UpdateVersion: "v1.2.3", Since: 4 * time.Second},
		},
		{
			name: "set healthy",
			request: newV2Request(&sessionv2.Request_SetHealthy{SetHealthy: &sessionv2.SetHealthyCommand{
				Components: []string{"gpu", "disk"},
				SinceNanos: int64(5 * time.Second),
			}}),
			want: Request{Method: "setHealthy", Components: []string{"gpu", "disk"}, Since: 5 * time.Second},
		},
		{
			name:    "reboot",
			request: newV2Request(&sessionv2.Request_Reboot{Reboot: &sessionv2.RebootCommand{}}),
			want:    Request{Method: "reboot"},
		},
		{
			name:    "update config",
			request: newV2Request(&sessionv2.Request_UpdateConfig{UpdateConfig: &sessionv2.UpdateConfigCommand{Values: map[string]string{"key": "value"}}}),
			want:    Request{Method: "updateConfig", UpdateConfig: map[string]string{"key": "value"}},
		},
		{
			name: "bootstrap",
			request: newV2Request(&sessionv2.Request_Bootstrap{Bootstrap: &sessionv2.BootstrapCommand{
				TimeoutSeconds: 42,
				ScriptBase64:   "c2NyaXB0",
				RequestPresent: true,
			}}),
			want: Request{Method: "bootstrap", Bootstrap: &BootstrapRequest{TimeoutInSeconds: 42, ScriptBase64: "c2NyaXB0"}},
		},
		{
			name:    "nil bootstrap",
			request: newV2Request(&sessionv2.Request_Bootstrap{Bootstrap: &sessionv2.BootstrapCommand{}}),
			want:    Request{Method: "bootstrap"},
		},
		{
			name: "inject fault",
			request: newV2Request(&sessionv2.Request_InjectFault{InjectFault: &sessionv2.InjectFaultCommand{
				RequestPresent: true,
				Fault:          &sessionv2.InjectFaultCommand_Xid{Xid: 79},
			}}),
			want: Request{Method: "injectFault", InjectFaultRequest: &pkgfaultinjector.Request{XID: &pkgfaultinjector.XIDToInject{ID: 79}}},
		},
		{
			name: "inject kernel message fault",
			request: newV2Request(&sessionv2.Request_InjectFault{InjectFault: &sessionv2.InjectFaultCommand{
				RequestPresent: true,
				Fault: &sessionv2.InjectFaultCommand_KernelMessage{KernelMessage: &sessionv2.KernelMessage{
					Priority: "KERN_WARNING",
					Message:  "test kernel fault",
				}},
			}}),
			want: Request{Method: "injectFault", InjectFaultRequest: &pkgfaultinjector.Request{KernelMessage: &pkgkmsgwriter.KernelMessage{
				Priority: pkgkmsgwriter.KernelMessagePriorityWarning,
				Message:  "test kernel fault",
			}}},
		},
		{
			name: "diagnostic",
			request: newV2Request(&sessionv2.Request_Diagnostic{Diagnostic: &sessionv2.DiagnosticCommand{
				ReportId:       "report-1",
				Type:           "gpu",
				TimeoutSeconds: 60,
				RequestPresent: true,
			}}),
			want: Request{Method: "diagnostic", Diagnostic: &DiagnosticRequest{ReportID: "report-1", Type: "gpu", TimeoutSeconds: 60}},
		},
		{
			name:    "nil diagnostic",
			request: newV2Request(&sessionv2.Request_Diagnostic{Diagnostic: &sessionv2.DiagnosticCommand{}}),
			want:    Request{Method: "diagnostic"},
		},
		{
			name:    "package status",
			request: newV2Request(&sessionv2.Request_GetPackageStatus{GetPackageStatus: &sessionv2.GetPackageStatusCommand{}}),
			want:    Request{Method: "packageStatus"},
		},
		{
			name:    "logout",
			request: newV2Request(&sessionv2.Request_Logout{Logout: &sessionv2.LogoutCommand{}}),
			want:    Request{Method: "logout"},
		},
		{
			name:    "gossip",
			request: newV2Request(&sessionv2.Request_Gossip{Gossip: &sessionv2.GossipCommand{}}),
			want:    Request{Method: "gossip"},
		},
		{
			name: "trigger component",
			request: newV2Request(&sessionv2.Request_TriggerComponent{TriggerComponent: &sessionv2.TriggerComponentCommand{
				ComponentName: "gpu",
				TagName:       "fast",
			}}),
			want: Request{Method: "triggerComponent", ComponentName: "gpu", TagName: "fast"},
		},
		{
			name: "plugin specs",
			request: newV2Request(&sessionv2.Request_SetPluginSpecs{SetPluginSpecs: &sessionv2.SetPluginSpecsCommand{
				SpecsPresent: true,
				Specs: []*sessionv2.PluginSpec{{
					PluginName:        "disk",
					PluginType:        "component",
					ComponentList:     []string{"nvme"},
					ComponentListFile: "/etc/components",
					RunMode:           "manual",
					Tags:              []string{"storage"},
					TimeoutNanos:      int64(10 * time.Second),
					IntervalNanos:     int64(time.Minute),
					HealthStatePlugin: &sessionv2.Plugin{
						Steps: []*sessionv2.PluginStep{{
							Name: "check",
							RunBashScript: &sessionv2.BashScript{
								ContentType: "plaintext",
								Script:      "echo ok",
							},
						}},
						Parser: &sessionv2.PluginOutputParser{
							LogPath: "/var/log/plugin.log",
							JsonPaths: []*sessionv2.PluginJSONPath{{
								Query:  "$.status",
								Field:  "status",
								Expect: &sessionv2.PluginMatchRule{Regex: &regex},
								SuggestedActions: map[string]*sessionv2.PluginMatchRule{
									"reboot": {Regex: &regex},
								},
							}},
						},
					},
				}},
			}}),
			want: Request{Method: "setPluginSpecs", CustomPluginSpecs: pkgcustomplugins.Specs{{
				PluginName:        "disk",
				PluginType:        "component",
				ComponentList:     []string{"nvme"},
				ComponentListFile: "/etc/components",
				RunMode:           "manual",
				Tags:              []string{"storage"},
				Timeout:           metav1.Duration{Duration: 10 * time.Second},
				Interval:          metav1.Duration{Duration: time.Minute},
				HealthStatePlugin: &pkgcustomplugins.Plugin{
					Steps: []pkgcustomplugins.Step{{
						Name: "check",
						RunBashScript: &pkgcustomplugins.RunBashScript{
							ContentType: "plaintext",
							Script:      "echo ok",
						},
					}},
					Parser: &pkgcustomplugins.PluginOutputParseConfig{
						LogPath: "/var/log/plugin.log",
						JSONPaths: []pkgcustomplugins.JSONPath{{
							Query:  "$.status",
							Field:  "status",
							Expect: &pkgcustomplugins.MatchRule{Regex: &regex},
							SuggestedActions: map[string]pkgcustomplugins.MatchRule{
								"reboot": {Regex: &regex},
							},
						}},
					},
				},
			}}},
		},
		{
			name:    "update token",
			request: newV2Request(&sessionv2.Request_UpdateToken{UpdateToken: &sessionv2.UpdateTokenCommand{Token: "token-1"}}),
			want:    Request{Method: "updateToken", Token: "token-1"},
		},
		{
			name:    "KAP status",
			request: newV2Request(&sessionv2.Request_GetKapMtlsStatus{GetKapMtlsStatus: &sessionv2.GetKAPMTLSStatusCommand{}}),
			want:    Request{Method: "kapMTLSStatus"},
		},
		{
			name: "KAP credentials",
			request: newV2Request(&sessionv2.Request_UpdateKapMtlsCredentials{UpdateKapMtlsCredentials: &sessionv2.UpdateKAPMTLSCredentialsCommand{
				CertificatePem:       []byte("cert"),
				PrivateKeyPem:        []byte("key"),
				GatewayCaPem:         []byte("ca"),
				GatewayEndpoint:      "gateway:8443",
				ServerName:           "gateway",
				ClientCaFingerprint:  "client-fingerprint",
				GatewayCaFingerprint: "gateway-fingerprint",
			}}),
			want: Request{Method: "updateKAPMTLSCredentials", KAPMTLSCredentials: &KAPMTLSCredentialsRequest{
				CertificatePEM:       []byte("cert"),
				PrivateKeyPEM:        []byte("key"),
				GatewayCAPEM:         []byte("ca"),
				GatewayEndpoint:      "gateway:8443",
				ServerName:           "gateway",
				ClientCAFingerprint:  "client-fingerprint",
				GatewayCAFingerprint: "gateway-fingerprint",
			}},
		},
		{
			name:    "activate KAP",
			request: newV2Request(&sessionv2.Request_ActivateKapMtls{ActivateKapMtls: &sessionv2.ActivateKAPMTLSCommand{}}),
			want:    Request{Method: "activateKAPMTLS"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := requestFromV2(test.request)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestRequestFromV2RejectsInvalidCommands(t *testing.T) {
	_, err := requestFromV2(nil)
	require.ErrorContains(t, err, "request is missing")

	_, err = requestFromV2(&sessionv2.Request{})
	require.ErrorContains(t, err, "command is missing")

	_, err = requestFromV2(newV2Request(&sessionv2.Request_GetEvents{}))
	require.ErrorContains(t, err, "get-events command is missing")

	for _, test := range []struct {
		name    string
		command any
		message string
	}{
		{name: "metrics", command: &sessionv2.Request_GetMetrics{}, message: "get-metrics command is missing"},
		{name: "update", command: &sessionv2.Request_Update{}, message: "update command is missing"},
		{name: "set healthy", command: &sessionv2.Request_SetHealthy{}, message: "set-healthy command is missing"},
		{name: "update config", command: &sessionv2.Request_UpdateConfig{}, message: "update-config command is missing"},
		{name: "bootstrap", command: &sessionv2.Request_Bootstrap{}, message: "bootstrap command is missing"},
		{name: "inject fault", command: &sessionv2.Request_InjectFault{}, message: "inject-fault command is missing"},
		{name: "diagnostic", command: &sessionv2.Request_Diagnostic{}, message: "diagnostic command is missing"},
		{name: "trigger component", command: &sessionv2.Request_TriggerComponent{}, message: "trigger-component command is missing"},
		{name: "plugin specs", command: &sessionv2.Request_SetPluginSpecs{}, message: "set-plugin-specs command is missing"},
		{name: "update token", command: &sessionv2.Request_UpdateToken{}, message: "update-token command is missing"},
		{name: "KAP credentials", command: &sessionv2.Request_UpdateKapMtlsCredentials{}, message: "update-KAP-mTLS-credentials command is missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := requestFromV2(newV2Request(test.command))
			require.ErrorContains(t, err, test.message)
		})
	}

	_, err = requestFromV2(newV2Request(&sessionv2.Request_GetEvents{GetEvents: &sessionv2.GetEventsCommand{}}))
	require.ErrorContains(t, err, "invalid get-events start time")

	_, err = requestFromV2(newV2Request(&sessionv2.Request_InjectFault{InjectFault: &sessionv2.InjectFaultCommand{
		Fault: &sessionv2.InjectFaultCommand_Xid{Xid: 79},
	}}))
	require.ErrorContains(t, err, "payload is present without a request")

	_, err = requestFromV2(newV2Request(&sessionv2.Request_InjectFault{InjectFault: &sessionv2.InjectFaultCommand{
		RequestPresent: true,
		Fault:          &sessionv2.InjectFaultCommand_KernelMessage{},
	}}))
	require.ErrorContains(t, err, "kernel-message fault is missing")

	_, err = requestFromV2(newV2Request(&sessionv2.Request_SetPluginSpecs{SetPluginSpecs: &sessionv2.SetPluginSpecsCommand{
		Specs: []*sessionv2.PluginSpec{{PluginName: "disk"}},
	}}))
	require.ErrorContains(t, err, "plugin specs are present without a specs payload")
}

func TestPluginSpecsFromV2PreservesNilEntries(t *testing.T) {
	got := pluginSpecsFromV2([]*sessionv2.PluginSpec{
		nil,
		{
			HealthStatePlugin: &sessionv2.Plugin{
				Steps: []*sessionv2.PluginStep{nil},
				Parser: &sessionv2.PluginOutputParser{JsonPaths: []*sessionv2.PluginJSONPath{
					nil,
					{SuggestedActions: map[string]*sessionv2.PluginMatchRule{"none": nil}},
				}},
			},
		},
	})

	require.Len(t, got, 2)
	assert.Equal(t, pkgcustomplugins.Spec{}, got[0])
	require.NotNil(t, got[1].HealthStatePlugin)
	assert.Equal(t, []pkgcustomplugins.Step{{}}, got[1].HealthStatePlugin.Steps)
	require.NotNil(t, got[1].HealthStatePlugin.Parser)
	assert.Equal(t, []pkgcustomplugins.JSONPath{{}, {SuggestedActions: map[string]pkgcustomplugins.MatchRule{}}}, got[1].HealthStatePlugin.Parser.JSONPaths)
	assert.Nil(t, pluginFromV2(nil))
	assert.Nil(t, pluginMatchRuleFromV2(nil))
}

func newV2Request(command any) *sessionv2.Request {
	request := &sessionv2.Request{RequestId: "request-1"}
	reflect.ValueOf(request).Elem().FieldByName("Command").Set(reflect.ValueOf(command))
	return request
}
