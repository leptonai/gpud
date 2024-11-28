package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	client_v1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/errdefs"
	"github.com/leptonai/gpud/log"
)

func main() {
	baseURL := "https://localhost:15132"
	componentName := "accelerator-nvidia-info"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	states, err := client_v1.GetStates(ctx, baseURL, client_v1.WithComponent(componentName))
	if err != nil {
		if errors.Is(err, errdefs.ErrNotFound) {
			log.Logger.Warnw("componentnot found", "component", componentName)
			return
		}

		log.Logger.Error("error fetching component info", "error", err)
		return
	}

	for _, ss := range states {
		for _, s := range ss.States {
			fmt.Printf("state: %q, healthy: %v, extra info: %q\n", s.Name, s.Healthy, s.ExtraInfo)
		}
	}
}
