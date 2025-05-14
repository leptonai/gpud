package command

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli"

	v1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/pkg/log"
)

// cmdRunPlugin implements the run command for plugin groups
func cmdRunPlugin(c *cli.Context) error {
	// Set up logging
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	// Get plugin group name from arguments
	if c.NArg() != 1 {
		return fmt.Errorf("exactly one argument (plugin_group_name) is required")
	}
	pluginGroupName := c.Args().Get(0)

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Trigger component check
	healthStates, err := v1.TriggerComponentCheck(ctx, "http://localhost:8080", pluginGroupName)
	if err != nil {
		return fmt.Errorf("failed to trigger component check: %w", err)
	}

	// Print health states
	fmt.Println("Component check results:")
	for _, state := range healthStates {
		fmt.Printf("- Component: %s\n", state.Component)
		fmt.Printf("  Health: %s\n", state.Health)
		fmt.Printf("  Reason: %s\n", state.Reason)
		if state.Error != "" {
			fmt.Printf("  Error: %s\n", state.Error)
		}
	}

	// Check if all components are healthy
	allHealthy := true
	for _, state := range healthStates {
		if state.Health != "Healthy" {
			allHealthy = false
			break
		}
	}

	// Exit with appropriate code
	if allHealthy {
		os.Exit(0)
	} else {
		os.Exit(1)
	}

	return nil
}
