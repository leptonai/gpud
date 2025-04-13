package main

import (
	"context"
	"fmt"
	"time"

	client_v1 "github.com/leptonai/gpud/client/v1"
)

func main() {
	baseURL := "https://localhost:15132"
	componentName := "" // Leave empty to query all components

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := client_v1.GetInfo(ctx, baseURL, client_v1.WithComponent(componentName))
	if err != nil {
		fmt.Println("Error fetching component info:", err)
		return
	}

	fmt.Println("Component Information:")
	for _, i := range info {
		fmt.Printf("Component: %s\n", i.Component)
		for _, event := range i.Info.Events {
			fmt.Printf("  Event: %s - %s\n", event.Name, event.Message)
		}
		for _, metric := range i.Info.Metrics {
			fmt.Printf("  Metric: %s - Value: %f\n", metric.DeprecatedMetricName, metric.Value)
		}
		for _, state := range i.Info.States {
			fmt.Printf("  State: %s - Healthy: %t\n", state.Name, state.DeprecatedHealthy)
		}
	}
}
