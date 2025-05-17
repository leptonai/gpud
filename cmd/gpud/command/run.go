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

	"github.com/leptonai/gpud/pkg/config"
	gpudmanager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/log"
	gpudserver "github.com/leptonai/gpud/pkg/server"
	pkgsystemd "github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/version"
)

// cmdRun implements the run command for starting the daemon
func cmdRun(cliContext *cli.Context) error {
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
	serverC := make(chan gpudserver.ServerStopper, 1)

	log.Logger.Infof("starting gpud %v", version.Version)

	done := gpudserver.HandleSignals(rootCtx, rootCancel, signals, serverC, func(ctx context.Context) error {
		if pkgsystemd.SystemctlExists() {
			if err := pkgsystemd.NotifyStopping(ctx); err != nil {
				log.Logger.Errorw("notify stopping failed")
			}
		}
		return nil
	})

	// start the signal handler as soon as we can to make sure that
	// we don't miss any signals during boot
	signal.Notify(signals, gpudserver.DefaultSignalsToHandle...)
	m, err := gpudmanager.New()
	if err != nil {
		return err
	}
	m.Start(rootCtx)

	server, err := gpudserver.New(rootCtx, cfg, m)
	if err != nil {
		return err
	}
	serverC <- server

	if pkgsystemd.SystemctlExists() {
		if err := pkgsystemd.NotifyReady(rootCtx); err != nil {
			log.Logger.Warnw("notify ready failed")
		}
	} else {
		log.Logger.Debugw("skipped sd notify as systemd is not available")
	}

	log.Logger.Infow("successfully booted", "tookSeconds", time.Since(start).Seconds())
	<-done

	return nil
}
