package main

import (
	"context"
	"time"

	nvidia_query "github.com/leptonai/gpud/pkg/nvidia-query"
	query_config "github.com/leptonai/gpud/pkg/query/config"
)

func main() {
	defaultQueryCfg := query_config.Config{
		State: &query_config.State{},
	}
	defaultQueryCfg.SetDefaultsIfNotSet()
	nvidia_query.SetDefaultPoller()
	nvidia_query.GetDefaultPoller().Start(context.Background(), defaultQueryCfg, "test")
	for {
		time.Sleep(1 * time.Second)
	}
}
