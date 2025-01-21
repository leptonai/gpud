package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/leptonai/gpud/components/accelerator/nvidia/infiniband"
	"github.com/leptonai/gpud/config"
	lepServer "github.com/leptonai/gpud/internal/server"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/manager"
	pkd_systemd "github.com/leptonai/gpud/pkg/systemd"
	"github.com/leptonai/gpud/version"

	"github.com/gin-gonic/gin"
	"github.com/urfave/cli"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func cmdRun(cliContext *cli.Context) error {
	if runtime.GOOS != "linux" {
		fmt.Printf("gpud run on %q not supported\n", runtime.GOOS)
		os.Exit(1)
	}

	var zapLvl zap.AtomicLevel = zap.NewAtomicLevel() // info level by default
	if logLevel != "" && logLevel != "info" {
		lCfg := log.DefaultLoggerConfig()
		var err error
		zapLvl, err = zap.ParseAtomicLevel(logLevel)
		if err != nil {
			return err
		}
		lCfg.Level = zapLvl
		log.Logger = log.CreateLogger(lCfg)
	}
	if zapLvl.Level() > zap.DebugLevel { // e.g., info, warn, error
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	configOpts := []config.OpOption{
		config.WithFilesToCheck(filesToCheck...),
		config.WithFilesToCheck(filesToCheck...),
		config.WithDockerIgnoreConnectionErrors(dockerIgnoreConnectionErrors),
		config.WithKubeletIgnoreConnectionErrors(kubeletIgnoreConnectionErrors),

		config.WithNvidiaSMICommand(nvidiaSMICommand),
		config.WithNvidiaSMIQueryCommand(nvidiaSMIQueryCommand),
		config.WithIbstatCommand(ibstatCommand),
		config.WithInfinibandClassDirectory(infinibandClassDirectory),
	}

	if expectedPortStates != "" {
		portStates := &infiniband.ExpectedPortStates{}
		if err := json.Unmarshal([]byte(expectedPortStates), portStates); err != nil {
			return err
		}
		configOpts = append(configOpts, config.WithExpectedPortStates(*portStates))
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
		cfg.Web.SincePeriod = metav1.Duration{Duration: retentionPeriod}
	}

	cfg.CompactPeriod = config.DefaultCompactPeriod

	cfg.Web.Enable = webEnable
	if webAdmin {
		cfg.Web.Admin = true
	}
	if webRefreshPeriod > 0 {
		cfg.Web.RefreshPeriod = metav1.Duration{Duration: webRefreshPeriod}
	}

	cfg.EnableAutoUpdate = enableAutoUpdate
	cfg.AutoUpdateExitCode = autoUpdateExitCode

	if err := cfg.Validate(); err != nil {
		return err
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()
	start := time.Now()

	signals := make(chan os.Signal, 2048)
	serverC := make(chan *lepServer.Server, 1)

	log.Logger.Infof("starting gpud %v", version.Version)

	done := handleSignals(rootCtx, rootCancel, signals, serverC)
	// start the signal handler as soon as we can to make sure that
	// we don't miss any signals during boot
	signal.Notify(signals, handledSignals...)
	m, err := manager.New()
	if err != nil {
		return err
	}
	m.Start(rootCtx)

	server, err := lepServer.New(rootCtx, cfg, cliContext.String("endpoint"), uid, m, configOpts...)
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
