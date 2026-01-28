package runplugingroup

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli"

	clientv1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
)

// Command implements the run-plugin-group command
func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	logFile := cliContext.String("log-file")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.CreateLogger(zapLvl, logFile))

	log.Logger.Debugw("starting run-plugin-group command")

	// Get the tag name from arguments
	if cliContext.NArg() != 1 {
		return fmt.Errorf("exactly one argument (tag_name) is required")
	}
	tagName := cliContext.Args().Get(0)

	// Get the server address from the flag, default to http://localhost:<Default GPUd port>
	serverAddr := cliContext.String("server")
	if serverAddr == "" {
		serverAddr = fmt.Sprintf("https://localhost:%d", config.GPUdPortNumber())
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Trigger the component check by tag
	err = clientv1.TriggerComponentCheckByTag(ctx, serverAddr, tagName)
	if err != nil {
		return fmt.Errorf("failed to trigger component check for tag %s: %w", tagName, err)
	}

	fmt.Printf("Successfully triggered component check for tag: %s\n", tagName)
	return nil
}
