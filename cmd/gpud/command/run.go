package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/pkg/config"
	gpud_manager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/log"
	gpudserver "github.com/leptonai/gpud/pkg/server"
	pkd_systemd "github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/version"
)

// cmdRun implements the run command
func cmdRun(c *cli.Context) error {
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

func cmdRunOld(cliContext *cli.Context) error {
	if runtime.GOOS != "linux" {
		fmt.Printf("gpud run on %q not supported\n", runtime.GOOS)
		os.Exit(1)
	}

	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	if zapLvl.Level() > zap.DebugLevel { // e.g., info, warn, error
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	configOpts := []config.OpOption{
		config.WithIbstatCommand(ibstatCommand),
		config.WithIbstatusCommand(ibstatusCommand),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	cfg, err := config.DefaultConfig(ctx, configOpts...)
	cancel()
	if err != nil {
		return err
	}

	if annotations != "" {
		annot := make(map[string]string)
		if err := json.Unmarshal([]byte(annotations), &annot); err != nil {
			return err
		}
		cfg.Annotations = annot
	}
	if listenAddress != "" {
		cfg.Address = listenAddress
	}
	if pprof {
		cfg.Pprof = true
	}
	if retentionPeriod > 0 {
		cfg.RetentionPeriod = metav1.Duration{Duration: retentionPeriod}
	}

	cfg.CompactPeriod = config.DefaultCompactPeriod

	cfg.EnableAutoUpdate = enableAutoUpdate
	cfg.AutoUpdateExitCode = autoUpdateExitCode

	cfg.PluginSpecsFile = pluginSpecsFile
	cfg.EnablePluginAPI = enablePluginAPI

	if err := cfg.Validate(); err != nil {
		return err
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	start := time.Now()

	signals := make(chan os.Signal, 2048)
	serverC := make(chan *gpudserver.Server, 1)

	log.Logger.Infof("starting gpud %v", version.Version)

	done := handleSignals(rootCtx, rootCancel, signals, serverC)
	// start the signal handler as soon as we can to make sure that
	// we don't miss any signals during boot
	signal.Notify(signals, handledSignals...)
	m, err := gpud_manager.New()
	if err != nil {
		return err
	}
	m.Start(rootCtx)

	server, err := gpudserver.New(rootCtx, cfg, m)
	if err != nil {
		return err
	}
	serverC <- server

	if pkd_systemd.SystemctlExists() {
		if err := notifyReady(rootCtx); err != nil {
			log.Logger.Warnw("notify ready failed")
		}
	} else {
		log.Logger.Debugw("skipped sd notify as systemd is not available")
	}

	log.Logger.Infow("successfully booted", "tookSeconds", time.Since(start).Seconds())
	<-done

	return nil
}
