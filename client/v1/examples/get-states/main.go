package main

import (
	"context"
	"time"

	client_v1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/log"
)

func main() {
	baseURL := "https://localhost:15132"
	for _, componentName := range []string{"disk", "accelerator-nvidia-info"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		states, err := client_v1.GetHealthStates(ctx, baseURL, client_v1.WithComponent(componentName))
		if err != nil {
			if errdefs.IsNotFound(err) {
				log.Logger.Warnw("component not found", "component", componentName)
				return
			}

			log.Logger.Error("error fetching component info", "error", err)
			return
		}

		for _, ss := range states {
			for _, s := range ss.States {
				log.Logger.Infof("state: %q, healthy: %v, extra info: %q\n", s.Name, s.DeprecatedHealthy, s.DeprecatedExtraInfo)
			}
		}
	}
}
