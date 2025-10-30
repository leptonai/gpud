package session

import (
	"context"
	"encoding/json"
	"time"

	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsnvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsnvidiainfinibanditypes "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband/types"
	componentsxid "github.com/leptonai/gpud/components/accelerator/nvidia/xid"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
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
			var updateCfg componentsxid.RebootThreshold
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal xid config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultXIDRebootThresholdFunc != nil {
				s.setDefaultXIDRebootThresholdFunc(updateCfg)
			}

		case componentsnfs.Name:
			setComponents[componentName] = struct{}{}
			var updateCfgs pkgnfschecker.Configs
			if err := json.Unmarshal([]byte(value), &updateCfgs); err != nil {
				log.Logger.Warnw("failed to unmarshal nfs config", "error", err)
				resp.Error = err.Error()
				return
			}

			// if NFS validation takes too long, it can block other session requests
			// so we set a timeout and do it async
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				err := updateCfgs.Validate(ctx)
				cancel()
				if err != nil {
					log.Logger.Warnw("invalid nfs config but proceeding with update to allow the user to fix the config", "error", err)
				}

				if s.setDefaultNFSGroupConfigsFunc != nil {
					s.setDefaultNFSGroupConfigsFunc(updateCfgs)
				}
			}()

		default:
			log.Logger.Warnw("unsupported component for updateConfig", "component", componentName)
		}
	}

	// fallback to default if the component is not set
	if _, ok := setComponents[componentsnvidiainfiniband.Name]; !ok && s.setDefaultIbExpectedPortStatesFunc != nil {
		log.Logger.Infow("falling back to default empty infiniband config")
		s.setDefaultIbExpectedPortStatesFunc(componentsnvidiainfinibanditypes.ExpectedPortStates{})
	}
	if _, ok := setComponents[componentsnvidiagpucounts.Name]; !ok && s.setDefaultGPUCountsFunc != nil {
		log.Logger.Infow("falling back to default empty nvidia gpu counts config")
		s.setDefaultGPUCountsFunc(componentsnvidiagpucounts.ExpectedGPUCounts{})
	}
	if _, ok := setComponents[componentsnfs.Name]; !ok && s.setDefaultNFSGroupConfigsFunc != nil {
		log.Logger.Infow("falling back to default empty nfs config")
		s.setDefaultNFSGroupConfigsFunc(pkgnfschecker.Configs{})
	}
	if _, ok := setComponents[componentsxid.Name]; !ok && s.setDefaultXIDRebootThresholdFunc != nil {
		log.Logger.Infow("falling back to default xid config")
		s.setDefaultXIDRebootThresholdFunc(componentsxid.RebootThreshold{Threshold: componentsxid.DefaultRebootThreshold})
	}
}
