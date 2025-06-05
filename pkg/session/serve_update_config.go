package session

import (
	"encoding/json"

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

		case componentsnfs.Name:
			var updateCfgs pkgnfschecker.Configs
			if err := json.Unmarshal([]byte(value), &updateCfgs); err != nil {
				log.Logger.Warnw("failed to unmarshal nfs config", "error", err)
				resp.Error = err.Error()
				return
			}
			if err := updateCfgs.Validate(); err != nil {
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
