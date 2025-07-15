package session

import (
	"context"
	"encoding/json"
	"time"

	componentsnvidiagpucounts "github.com/leptonai/gpud/components/accelerator/nvidia/gpu-counts"
	componentsnvidiainfiniband "github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	componentsnfs "github.com/leptonai/gpud/components/nfs"
	"github.com/leptonai/gpud/pkg/log"
	pkgnfschecker "github.com/leptonai/gpud/pkg/nfs-checker"
	"github.com/leptonai/gpud/pkg/nvidia-query/infiniband"
)

func (s *Session) processUpdateConfig(configMap map[string]string, resp *Response) {
	if len(configMap) == 0 {
		return
	}

	for componentName, value := range configMap {
		log.Logger.Infow("processing update config request", "component", componentName, "config", value)

		switch componentName {
		case componentsnvidiainfiniband.Name:
			var updateCfg infiniband.ExpectedPortStates
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal infiniband config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultIbExpectedPortStatesFunc != nil {
				s.setDefaultIbExpectedPortStatesFunc(updateCfg)
			}

		case componentsnvidiagpucounts.Name:
			var updateCfg componentsnvidiagpucounts.ExpectedGPUCounts
			if err := json.Unmarshal([]byte(value), &updateCfg); err != nil {
				log.Logger.Warnw("failed to unmarshal nvidia gpu counts config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultGPUCountsFunc != nil {
				s.setDefaultGPUCountsFunc(updateCfg)
			}

		case componentsnfs.Name:
			var updateCfgs pkgnfschecker.Configs
			if err := json.Unmarshal([]byte(value), &updateCfgs); err != nil {
				log.Logger.Warnw("failed to unmarshal nfs config", "error", err)
				resp.Error = err.Error()
				return
			}
			// Create a context with timeout for validation
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := updateCfgs.Validate(ctx)
			cancel()
			if err != nil {
				log.Logger.Warnw("invalid nfs config", "error", err)
				resp.Error = err.Error()
				return
			}
			if s.setDefaultNFSGroupConfigsFunc != nil {
				s.setDefaultNFSGroupConfigsFunc(updateCfgs)
			}

		default:
			log.Logger.Warnw("unsupported component for updateConfig", "component", componentName)
		}
	}
}
