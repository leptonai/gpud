package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"time"

	"github.com/leptonai/gpud/pkg/config"
	gpud_manager "github.com/leptonai/gpud/pkg/gpud-manager"
	"github.com/leptonai/gpud/pkg/log"
	lepServer "github.com/leptonai/gpud/pkg/server"
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
		config.WithDockerIgnoreConnectionErrors(dockerIgnoreConnectionErrors),
		config.WithKubeletIgnoreConnectionErrors(kubeletIgnoreConnectionErrors),
		config.WithIbstatCommand(ibstatCommand),
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

	cfg.EnableGlobalMetricsStore = experimentalGlobalMetricsStore

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
	m, err := gpud_manager.New()
	if err != nil {
		return err
	}
	m.Start(rootCtx)

	server, err := lepServer.New(rootCtx, cfg, cliContext.String("endpoint"), uid, m)
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
