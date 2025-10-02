package sethealthy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli"

	clientv1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
)

// CreateCommand creates the set-healthy command
func CreateCommand() func(*cli.Context) error {
	return func(cliContext *cli.Context) error {
		logLevel := cliContext.String("log-level")
		logFile := cliContext.String("log-file")
		zapLvl, err := log.ParseLogLevel(logLevel)
		if err != nil {
			return err
		}
		log.Logger = log.CreateLogger(zapLvl, logFile)

		log.Logger.Debugw("starting set-healthy command")

		// Get the server address from the flag, default to https://localhost:<Default GPUd port>
		serverAddr := cliContext.String("server")
		if serverAddr == "" {
			serverAddr = fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort)
		}

		// Get the components from the positional argument
		var components []string
		if cliContext.NArg() > 0 {
			componentsStr := cliContext.Args().Get(0)
			if componentsStr != "" {
				components = strings.Split(componentsStr, ",")
				// Trim spaces from component names
				for i := range components {
					components[i] = strings.TrimSpace(components[i])
				}
			}
		}

		// Create a context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Call the API to set components healthy
		res, err := clientv1.SetHealthyComponents(ctx, serverAddr, components)
		if err != nil {
			return fmt.Errorf("failed to set components healthy: %w", err)
		}

		fmt.Printf("successfully set components to healthy: %s\n", strings.Join(res, ", "))
		return nil
	}
}
