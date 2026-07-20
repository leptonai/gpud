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

func requestFromV2(request *sessionv2.Request) (Request, error) {
	if request == nil {
		return Request{}, fmt.Errorf("v2 request is missing")
	}

	var legacy Request
	switch command := request.Command.(type) {
	case *sessionv2.Request_GetHealthStates:
		legacy.Method = "states"
	case *sessionv2.Request_GetEvents:
		if command.GetEvents == nil {
			return Request{}, fmt.Errorf("get-events command is missing")
		}
		if err := command.GetEvents.StartTime.CheckValid(); err != nil {
			return Request{}, fmt.Errorf("invalid get-events start time: %w", err)
		}
		if err := command.GetEvents.EndTime.CheckValid(); err != nil {
			return Request{}, fmt.Errorf("invalid get-events end time: %w", err)
		}
		legacy.Method = "events"
		legacy.StartTime = command.GetEvents.StartTime.AsTime()
		legacy.EndTime = command.GetEvents.EndTime.AsTime()
	case *sessionv2.Request_GetMetrics:
		if command.GetMetrics == nil {
			return Request{}, fmt.Errorf("get-metrics command is missing")
		}
		legacy.Method = "metrics"
		legacy.Since = time.Duration(command.GetMetrics.SinceNanos)
	case *sessionv2.Request_Update:
		if command.Update == nil {
			return Request{}, fmt.Errorf("update command is missing")
		}
		legacy.Method = "update"
		legacy.UpdateVersion = command.Update.Version
		legacy.Since = time.Duration(command.Update.SinceNanos)
	case *sessionv2.Request_SetHealthy:
		if command.SetHealthy == nil {
			return Request{}, fmt.Errorf("set-healthy command is missing")
		}
		legacy.Method = "setHealthy"
		legacy.Components = command.SetHealthy.Components
		legacy.Since = time.Duration(command.SetHealthy.SinceNanos)
	case *sessionv2.Request_Reboot:
		legacy.Method = "reboot"
	case *sessionv2.Request_UpdateConfig:
		if command.UpdateConfig == nil {
			return Request{}, fmt.Errorf("update-config command is missing")
		}
		legacy.Method = "updateConfig"
		legacy.UpdateConfig = command.UpdateConfig.Values
	case *sessionv2.Request_Bootstrap:
		if command.Bootstrap == nil {
			return Request{}, fmt.Errorf("bootstrap command is missing")
		}
		legacy.Method = "bootstrap"
		if command.Bootstrap.RequestPresent {
			legacy.Bootstrap = &BootstrapRequest{
				TimeoutInSeconds: int(command.Bootstrap.TimeoutSeconds),
				ScriptBase64:     command.Bootstrap.ScriptBase64,
			}
		}
	case *sessionv2.Request_InjectFault:
		if command.InjectFault == nil {
			return Request{}, fmt.Errorf("inject-fault command is missing")
		}
		legacy.Method = "injectFault"
		if command.InjectFault.RequestPresent {
			legacy.InjectFaultRequest = &pkgfaultinjector.Request{}
			switch fault := command.InjectFault.Fault.(type) {
			case *sessionv2.InjectFaultCommand_Xid:
				legacy.InjectFaultRequest.XID = &pkgfaultinjector.XIDToInject{ID: int(fault.Xid)}
			case *sessionv2.InjectFaultCommand_KernelMessage:
				if fault.KernelMessage == nil {
					return Request{}, fmt.Errorf("kernel-message fault is missing")
				}
				legacy.InjectFaultRequest.KernelMessage = &pkgkmsgwriter.KernelMessage{
					Priority: pkgkmsgwriter.KernelMessagePriority(fault.KernelMessage.Priority),
					Message:  fault.KernelMessage.Message,
				}
			}
		} else if command.InjectFault.Fault != nil {
			return Request{}, fmt.Errorf("inject-fault payload is present without a request")
		}
	case *sessionv2.Request_Diagnostic:
		if command.Diagnostic == nil {
			return Request{}, fmt.Errorf("diagnostic command is missing")
		}
		legacy.Method = "diagnostic"
		if command.Diagnostic.RequestPresent {
			legacy.Diagnostic = &DiagnosticRequest{
				ReportID:       command.Diagnostic.ReportId,
				Type:           command.Diagnostic.Type,
				TimeoutSeconds: command.Diagnostic.TimeoutSeconds,
			}
		}
	case *sessionv2.Request_GetPackageStatus:
		legacy.Method = "packageStatus"
	case *sessionv2.Request_Logout:
		legacy.Method = "logout"
	case *sessionv2.Request_Gossip:
		legacy.Method = "gossip"
	case *sessionv2.Request_TriggerComponent:
		if command.TriggerComponent == nil {
			return Request{}, fmt.Errorf("trigger-component command is missing")
		}
		legacy.Method = "triggerComponent"
		legacy.ComponentName = command.TriggerComponent.ComponentName
		legacy.TagName = command.TriggerComponent.TagName
	case *sessionv2.Request_SetPluginSpecs:
		if command.SetPluginSpecs == nil {
			return Request{}, fmt.Errorf("set-plugin-specs command is missing")
		}
		legacy.Method = "setPluginSpecs"
		if command.SetPluginSpecs.SpecsPresent {
			legacy.CustomPluginSpecs = pluginSpecsFromV2(command.SetPluginSpecs.Specs)
		} else if len(command.SetPluginSpecs.Specs) != 0 {
			return Request{}, fmt.Errorf("plugin specs are present without a specs payload")
		}
	case *sessionv2.Request_UpdateToken:
		if command.UpdateToken == nil {
			return Request{}, fmt.Errorf("update-token command is missing")
		}
		legacy.Method = "updateToken"
		legacy.Token = command.UpdateToken.Token
	case *sessionv2.Request_GetKapMtlsStatus:
		legacy.Method = "kapMTLSStatus"
	case *sessionv2.Request_UpdateKapMtlsCredentials:
		if command.UpdateKapMtlsCredentials == nil {
			return Request{}, fmt.Errorf("update-KAP-mTLS-credentials command is missing")
		}
		credentials := command.UpdateKapMtlsCredentials
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
	case *sessionv2.Request_ActivateKapMtls:
		legacy.Method = "activateKAPMTLS"
	case nil:
		return Request{}, fmt.Errorf("v2 request command is missing")
	default:
		return Request{}, fmt.Errorf("unsupported v2 request command %T", command)
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
