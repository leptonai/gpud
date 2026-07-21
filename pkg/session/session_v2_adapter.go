// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pkgcustomplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	pkgfaultinjector "github.com/leptonai/gpud/pkg/fault-injector"
	pkgkmsgwriter "github.com/leptonai/gpud/pkg/kmsg/writer"
	sessionv2 "github.com/leptonai/gpud/pkg/session/v2"
)

func requestFromV2(packet *sessionv2.ManagerPacket) (Request, error) {
	if packet == nil {
		return Request{}, fmt.Errorf("v2 request is missing")
	}

	var legacy Request
	switch payload := packet.Payload.(type) {
	case *sessionv2.ManagerPacket_GetHealthStates:
		if payload.GetHealthStates == nil {
			return Request{}, fmt.Errorf("get-health-states payload is missing")
		}
		legacy.Method = "states"
	case *sessionv2.ManagerPacket_GetEvents:
		if payload.GetEvents == nil {
			return Request{}, fmt.Errorf("get-events payload is missing")
		}
		if err := payload.GetEvents.StartTime.CheckValid(); err != nil {
			return Request{}, fmt.Errorf("invalid get-events start time: %w", err)
		}
		if err := payload.GetEvents.EndTime.CheckValid(); err != nil {
			return Request{}, fmt.Errorf("invalid get-events end time: %w", err)
		}
		legacy.Method = "events"
		legacy.StartTime = payload.GetEvents.StartTime.AsTime()
		legacy.EndTime = payload.GetEvents.EndTime.AsTime()
	case *sessionv2.ManagerPacket_GetMetrics:
		if payload.GetMetrics == nil {
			return Request{}, fmt.Errorf("get-metrics payload is missing")
		}
		legacy.Method = "metrics"
		legacy.Since = time.Duration(payload.GetMetrics.SinceNanos)
	case *sessionv2.ManagerPacket_Update:
		if payload.Update == nil {
			return Request{}, fmt.Errorf("update payload is missing")
		}
		legacy.Method = "update"
		legacy.UpdateVersion = payload.Update.Version
		legacy.Since = time.Duration(payload.Update.SinceNanos)
	case *sessionv2.ManagerPacket_SetHealthy:
		if payload.SetHealthy == nil {
			return Request{}, fmt.Errorf("set-healthy payload is missing")
		}
		legacy.Method = "setHealthy"
		legacy.Components = payload.SetHealthy.Components
		legacy.Since = time.Duration(payload.SetHealthy.SinceNanos)
	case *sessionv2.ManagerPacket_Reboot:
		if payload.Reboot == nil {
			return Request{}, fmt.Errorf("reboot payload is missing")
		}
		legacy.Method = "reboot"
	case *sessionv2.ManagerPacket_UpdateConfig:
		if payload.UpdateConfig == nil {
			return Request{}, fmt.Errorf("update-config payload is missing")
		}
		legacy.Method = "updateConfig"
		legacy.UpdateConfig = payload.UpdateConfig.Values
	case *sessionv2.ManagerPacket_Bootstrap:
		if payload.Bootstrap == nil {
			return Request{}, fmt.Errorf("bootstrap payload is missing")
		}
		legacy.Method = "bootstrap"
		if payload.Bootstrap.RequestPresent {
			legacy.Bootstrap = &BootstrapRequest{
				TimeoutInSeconds: int(payload.Bootstrap.TimeoutSeconds),
				ScriptBase64:     payload.Bootstrap.ScriptBase64,
			}
		}
	case *sessionv2.ManagerPacket_InjectFault:
		if payload.InjectFault == nil {
			return Request{}, fmt.Errorf("inject-fault payload is missing")
		}
		legacy.Method = "injectFault"
		if payload.InjectFault.RequestPresent {
			legacy.InjectFaultRequest = &pkgfaultinjector.Request{}
			switch fault := payload.InjectFault.Fault.(type) {
			case *sessionv2.InjectFaultRequest_Xid:
				legacy.InjectFaultRequest.XID = &pkgfaultinjector.XIDToInject{ID: int(fault.Xid)}
			case *sessionv2.InjectFaultRequest_KernelMessage:
				if fault.KernelMessage == nil {
					return Request{}, fmt.Errorf("kernel-message fault is missing")
				}
				legacy.InjectFaultRequest.KernelMessage = &pkgkmsgwriter.KernelMessage{
					Priority: pkgkmsgwriter.KernelMessagePriority(fault.KernelMessage.Priority),
					Message:  fault.KernelMessage.Message,
				}
			}
		} else if payload.InjectFault.Fault != nil {
			return Request{}, fmt.Errorf("inject-fault payload is present without a request")
		}
	case *sessionv2.ManagerPacket_Diagnostic:
		if payload.Diagnostic == nil {
			return Request{}, fmt.Errorf("diagnostic payload is missing")
		}
		legacy.Method = "diagnostic"
		if payload.Diagnostic.RequestPresent {
			legacy.Diagnostic = &DiagnosticRequest{
				ReportID:       payload.Diagnostic.ReportId,
				Type:           payload.Diagnostic.Type,
				TimeoutSeconds: payload.Diagnostic.TimeoutSeconds,
			}
		}
	case *sessionv2.ManagerPacket_GetPackageStatus:
		if payload.GetPackageStatus == nil {
			return Request{}, fmt.Errorf("get-package-status payload is missing")
		}
		legacy.Method = "packageStatus"
	case *sessionv2.ManagerPacket_Logout:
		if payload.Logout == nil {
			return Request{}, fmt.Errorf("logout payload is missing")
		}
		legacy.Method = "logout"
	case *sessionv2.ManagerPacket_Gossip:
		if payload.Gossip == nil {
			return Request{}, fmt.Errorf("gossip payload is missing")
		}
		legacy.Method = "gossip"
	case *sessionv2.ManagerPacket_TriggerComponent:
		if payload.TriggerComponent == nil {
			return Request{}, fmt.Errorf("trigger-component payload is missing")
		}
		legacy.Method = "triggerComponent"
		legacy.ComponentName = payload.TriggerComponent.ComponentName
		legacy.TagName = payload.TriggerComponent.TagName
	case *sessionv2.ManagerPacket_SetPluginSpecs:
		if payload.SetPluginSpecs == nil {
			return Request{}, fmt.Errorf("set-plugin-specs payload is missing")
		}
		legacy.Method = "setPluginSpecs"
		if payload.SetPluginSpecs.SpecsPresent {
			legacy.CustomPluginSpecs = pluginSpecsFromV2(payload.SetPluginSpecs.Specs)
		} else if len(payload.SetPluginSpecs.Specs) != 0 {
			return Request{}, fmt.Errorf("plugin specs are present without a specs payload")
		}
	case *sessionv2.ManagerPacket_UpdateToken:
		if payload.UpdateToken == nil {
			return Request{}, fmt.Errorf("update-token payload is missing")
		}
		legacy.Method = "updateToken"
		legacy.Token = payload.UpdateToken.Token
	case *sessionv2.ManagerPacket_GetKapMtlsStatus:
		if payload.GetKapMtlsStatus == nil {
			return Request{}, fmt.Errorf("get-KAP-mTLS-status payload is missing")
		}
		legacy.Method = "kapMTLSStatus"
	case *sessionv2.ManagerPacket_UpdateKapMtlsCredentials:
		if payload.UpdateKapMtlsCredentials == nil {
			return Request{}, fmt.Errorf("update-KAP-mTLS-credentials payload is missing")
		}
		credentials := payload.UpdateKapMtlsCredentials
		legacy.Method = "updateKAPMTLSCredentials"
		legacy.KAPMTLSCredentials = &KAPMTLSCredentialsRequest{
			CertificatePEM:       credentials.CertificatePem,
			PrivateKeyPEM:        credentials.PrivateKeyPem,
			GatewayCAPEM:         credentials.GatewayCaPem,
			GatewayEndpoint:      credentials.GatewayEndpoint,
			ServerName:           credentials.ServerName,
			ClientCAFingerprint:  credentials.ClientCaFingerprint,
			GatewayCAFingerprint: credentials.GatewayCaFingerprint,
		}
	case *sessionv2.ManagerPacket_ActivateKapMtls:
		if payload.ActivateKapMtls == nil {
			return Request{}, fmt.Errorf("activate-KAP-mTLS payload is missing")
		}
		legacy.Method = "activateKAPMTLS"
	case nil:
		return Request{}, fmt.Errorf("v2 request payload is missing")
	default:
		return Request{}, fmt.Errorf("unsupported v2 request payload %T", payload)
	}

	return legacy, nil
}

func pluginSpecsFromV2(specs []*sessionv2.PluginSpec) pkgcustomplugins.Specs {
	converted := make(pkgcustomplugins.Specs, 0, len(specs))
	for _, spec := range specs {
		if spec == nil {
			converted = append(converted, pkgcustomplugins.Spec{})
			continue
		}
		converted = append(converted, pkgcustomplugins.Spec{
			PluginName:        spec.PluginName,
			PluginType:        spec.PluginType,
			ComponentList:     spec.ComponentList,
			ComponentListFile: spec.ComponentListFile,
			RunMode:           spec.RunMode,
			Tags:              spec.Tags,
			HealthStatePlugin: pluginFromV2(spec.HealthStatePlugin),
			Timeout:           metav1.Duration{Duration: time.Duration(spec.TimeoutNanos)},
			Interval:          metav1.Duration{Duration: time.Duration(spec.IntervalNanos)},
		})
	}
	return converted
}

func pluginFromV2(plugin *sessionv2.Plugin) *pkgcustomplugins.Plugin {
	if plugin == nil {
		return nil
	}
	converted := &pkgcustomplugins.Plugin{Steps: make([]pkgcustomplugins.Step, 0, len(plugin.Steps))}
	for _, step := range plugin.Steps {
		if step == nil {
			converted.Steps = append(converted.Steps, pkgcustomplugins.Step{})
			continue
		}
		convertedStep := pkgcustomplugins.Step{Name: step.Name}
		if step.RunBashScript != nil {
			convertedStep.RunBashScript = &pkgcustomplugins.RunBashScript{
				ContentType: step.RunBashScript.ContentType,
				Script:      step.RunBashScript.Script,
			}
		}
		converted.Steps = append(converted.Steps, convertedStep)
	}
	if plugin.Parser != nil {
		converted.Parser = &pkgcustomplugins.PluginOutputParseConfig{
			JSONPaths: make([]pkgcustomplugins.JSONPath, 0, len(plugin.Parser.JsonPaths)),
			LogPath:   plugin.Parser.LogPath,
		}
		for _, path := range plugin.Parser.JsonPaths {
			if path == nil {
				converted.Parser.JSONPaths = append(converted.Parser.JSONPaths, pkgcustomplugins.JSONPath{})
				continue
			}
			convertedPath := pkgcustomplugins.JSONPath{
				Query:            path.Query,
				Field:            path.Field,
				Expect:           pluginMatchRuleFromV2(path.Expect),
				SuggestedActions: make(map[string]pkgcustomplugins.MatchRule, len(path.SuggestedActions)),
			}
			for action, rule := range path.SuggestedActions {
				convertedRule := pluginMatchRuleFromV2(rule)
				if convertedRule != nil {
					convertedPath.SuggestedActions[action] = *convertedRule
				}
			}
			converted.Parser.JSONPaths = append(converted.Parser.JSONPaths, convertedPath)
		}
	}
	return converted
}

func pluginMatchRuleFromV2(rule *sessionv2.PluginMatchRule) *pkgcustomplugins.MatchRule {
	if rule == nil {
		return nil
	}
	return &pkgcustomplugins.MatchRule{Regex: rule.Regex}
}
