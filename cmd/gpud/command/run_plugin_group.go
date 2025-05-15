package command

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli"

	v1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/pkg/config"
)

// cmdRunPluginGroup implements the run-plugin-group command
func cmdRunPluginGroup(c *cli.Context) error {
	// Get the tag name from arguments
	if c.NArg() != 1 {
		return fmt.Errorf("exactly one argument (tag_name) is required")
	}
	tagName := c.Args().Get(0)

	// Get the server address from the flag, default to http://localhost:<Default GPUd port>
	serverAddr := c.String("server")
	if serverAddr == "" {
		serverAddr = fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Trigger the component check by tag
	err := v1.TriggerComponentCheckByTag(ctx, serverAddr, tagName)
	if err != nil {
		return fmt.Errorf("failed to trigger component check for tag %s: %w", tagName, err)
	}

	fmt.Printf("Successfully triggered component check for tag: %s\n", tagName)
	return nil
}
