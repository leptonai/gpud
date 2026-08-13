package session

import (
	"encoding/json"

	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsnvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsnvidiainfinibanditypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	componentsnvidianvlink "github.com/leptonai/gpud/components/accelerator/nvidia/nvlink"
	componentstemperature "github.com/leptonai/gpud/components/accelerator/nvidia/temperature"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	componentsos "github.com/leptonai/gpud/components/os"
	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
)

func (s *Session) processUpdateConfig(configMap map[string]string, resp *Response) {
	if len(configMap) == 0 {
		return
	}

	setComponents := make(map[string]any)
	for componentName, value := range configMap {
		log.Logger.Infow("processing update config request", "component", componentName, "config", value)

		switch componentName {
		case componentsnvidiainfiniband.Name:
			setComponents[componentName] = struct{}{}
			var updateCfg componentsnvidiainfinibanditypes.ExpectedPortStates
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal infiniband config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultIbExpectedPortStatesFunc != nil {
				s.setDefaultIbExpectedPortStatesFunc(updateCfg)
			}

		case componentsnvidianvlink.Name:
			setComponents[componentName] = struct{}{}
			var updateCfg componentsnvidianvlink.ExpectedLinkStates
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal nvlink config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultNVLinkExpectedLinkStatesFunc != nil {
				s.setDefaultNVLinkExpectedLinkStatesFunc(updateCfg)
			}

		case componentsnvidiagpucounts.Name:
			setComponents[componentName] = struct{}{}
			var updateCfg componentsnvidiagpucounts.ExpectedGPUCounts
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal nvidia gpu counts config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultGPUCountsFunc != nil {
				s.setDefaultGPUCountsFunc(updateCfg)
			}

		case componentsxid.Name:
			setComponents[componentName] = struct{}{}
			var updateCfg componentsxid.Thresholds
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal xid config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultXIDThresholdsFunc != nil {
				s.setDefaultXIDThresholdsFunc(updateCfg)
			}

		case componentstemperature.Name:
			setComponents[componentName] = struct{}{}
			var updateCfg componentstemperature.Thresholds
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal temperature config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultTemperatureThresholdsFunc != nil {
				s.setDefaultTemperatureThresholdsFunc(updateCfg)
			}

		case componentsnfs.Name:
			setComponents[componentName] = struct{}{}
			var updateCfgs pkgnfschecker.Configs
			if err := json.Unmarshal([]byte(value), &updateCfgs); err != nil {
				log.Logger.Warnw("failed to unmarshal nfs config", "error", err)
				resp.Error = err.Error()
				return
			}

			// Path validation belongs to the NFS component, which has the deployment's
			// host-root view. Validating here would inspect the container namespace and
			// incorrectly reject valid BYOK node paths before /proc/1/root is applied.
			if s.setDefaultNFSGroupConfigsFunc != nil {
				s.setDefaultNFSGroupConfigsFunc(updateCfgs)
			}

		case componentsos.Name:
			setComponents[componentName] = struct{}{}
			var updateCfg componentsos.BlockedProcessThresholds
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal os blocked process thresholds config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultOSBlockedProcessThresholdsFunc != nil {
				if err := s.setDefaultOSBlockedProcessThresholdsFunc(updateCfg); err != nil {
					log.Logger.Warnw("failed to set os blocked process thresholds", "error", err)
					resp.Error = err.Error()
					return
				}
			}

		default:
			log.Logger.Warnw("unsupported component for updateConfig", "component", componentName)
		}
	}

	// fallback to default if the component is not set
	if _, ok := setComponents[componentsnvidiainfiniband.Name]; !ok && s.setDefaultIbExpectedPortStatesFunc != nil {
		log.Logger.Infow("falling back to default empty infiniband config")
		s.setDefaultIbExpectedPortStatesFunc(componentsnvidiainfinibanditypes.ExpectedPortStates{})
	}
	if _, ok := setComponents[componentsnvidianvlink.Name]; !ok && s.setDefaultNVLinkExpectedLinkStatesFunc != nil {
		log.Logger.Infow("falling back to default empty nvlink config")
		s.setDefaultNVLinkExpectedLinkStatesFunc(componentsnvidianvlink.ExpectedLinkStates{})
	}
	if _, ok := setComponents[componentsnvidiagpucounts.Name]; !ok && s.setDefaultGPUCountsFunc != nil {
		log.Logger.Infow("falling back to default empty nvidia gpu counts config")
		s.setDefaultGPUCountsFunc(componentsnvidiagpucounts.ExpectedGPUCounts{})
	}
	if _, ok := setComponents[componentsnfs.Name]; !ok && s.setDefaultNFSGroupConfigsFunc != nil {
		log.Logger.Infow("falling back to default empty nfs config")
		s.setDefaultNFSGroupConfigsFunc(pkgnfschecker.Configs{})
	}
	if _, ok := setComponents[componentsxid.Name]; !ok && s.setDefaultXIDThresholdsFunc != nil {
		log.Logger.Infow("falling back to default xid config")
		s.setDefaultXIDThresholdsFunc(componentsxid.Thresholds{})
	}
	if _, ok := setComponents[componentstemperature.Name]; !ok && s.setDefaultTemperatureThresholdsFunc != nil {
		log.Logger.Infow("falling back to default temperature config")
		s.setDefaultTemperatureThresholdsFunc(componentstemperature.Thresholds{CelsiusSlowdownMargin: componentstemperature.ThresholdCelsiusSlowdownMargin})
	}
	// os falls back to the startup-resolved thresholds (CLI flags + NVIDIA GPU
	// auto-detection), not the built-in empty default: resetting to empty here
	// would silently disable D-state tracking on GPU machines whenever the
	// control plane pushes a config that does not mention the os component.
	if _, ok := setComponents[componentsos.Name]; !ok && s.setDefaultOSBlockedProcessThresholdsFunc != nil {
		log.Logger.Infow("falling back to startup os blocked process thresholds")
		if err := s.setDefaultOSBlockedProcessThresholdsFunc(componentsos.GetStartupBlockedProcessThresholds()); err != nil {
			// the startup baseline was validated when recorded; log defensively
			log.Logger.Warnw("failed to restore startup os blocked process thresholds", "error", err)
		}
	}
}
