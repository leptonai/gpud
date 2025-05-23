package listplugins

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli"

	clientv1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
)

// Command implements the list-plugins command
func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting list-plugins command")

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get the server address from the flag, default to http://localhost:<Default GPUd port>
	serverAddr := cliContext.String("server")
	if serverAddr == "" {
		serverAddr = fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort)
	}

	// Get custom plugins
	plugins, err := clientv1.GetCustomPlugins(ctx, serverAddr)
	if err != nil {
		return fmt.Errorf("failed to get custom plugins: %w", err)
	}

	// Print plugins
	if len(plugins) == 0 {
		fmt.Println("No custom plugins registered")
		return nil
	}

	fmt.Println("Registered custom plugins:")
	for name, spec := range plugins {
		fmt.Printf("- %s (Type: %s, Run Mode: %s)\n", name, spec.Type, spec.RunMode)
	}

	return nil
}
