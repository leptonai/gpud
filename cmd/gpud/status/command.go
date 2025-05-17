package status

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/urfave/cli"

	clientv1 "github.com/leptonai/gpud/client/v1"
	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/errdefs"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/server"
	"github.com/leptonai/gpud/pkg/systemd"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	var active bool
	if systemd.SystemctlExists() {
		active, err = systemd.IsActive("gpud.service")
		if err != nil {
			return err
		}
		if !active {
			fmt.Printf("%s gpud.service is not active\n", cmdcommon.WarningSign)
		} else {
			fmt.Printf("%s gpud.service is active\n", cmdcommon.CheckMark)
		}
	}
	if !active {
		// fallback to process list
		// in case it's not using systemd
		proc, err := process.FindProcessByName(rootCtx, "gpud")
		if err != nil {
			return err
		}
		if proc == nil {
			fmt.Printf("%s gpud process is not running\n", cmdcommon.WarningSign)
			return nil
		}

		fmt.Printf("%s gpud process is running (PID %d)\n", cmdcommon.CheckMark, proc.PID())
	}
	fmt.Printf("%s successfully checked gpud status\n", cmdcommon.CheckMark)

	if err := checkDiskComponent(); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked whether disk component is running\n", cmdcommon.CheckMark)

	if err := checkNvidiaInfoComponent(); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked whether accelerator-nvidia-info component is running\n", cmdcommon.CheckMark)

	if err := clientv1.BlockUntilServerReady(
		rootCtx,
		fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort),
	); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked gpud health\n", cmdcommon.CheckMark)

	statusWatch := cliContext.Bool("watch")

	for {
		cctx, ccancel := context.WithTimeout(rootCtx, 15*time.Second)
		packageStatus, err := clientv1.GetPackageStatus(cctx, fmt.Sprintf("https://localhost:%d%s", config.DefaultGPUdPort, server.URLPathAdminPackages))
		ccancel()
		if err != nil {
			fmt.Printf("%s failed to get package status: %v\n", cmdcommon.WarningSign, err)
			return err
		}
		if len(packageStatus) == 0 {
			fmt.Printf("no packages found\n")
			return nil
		}
		if statusWatch {
			fmt.Print("\033[2J\033[H")
		}
		var totalTime int64
		var progress int64
		for _, status := range packageStatus {
			totalTime += status.TotalTime.Milliseconds()
			progress += status.TotalTime.Milliseconds() * int64(status.Progress) / 100
		}

		var totalProgress int64
		if totalTime != 0 {
			totalProgress = progress * 100 / totalTime
		}
		fmt.Printf("Total progress: %v%%, Estimate time left: %v\n", totalProgress, time.Duration(totalTime-progress)*time.Millisecond)
		if !statusWatch {
			break
		}
		time.Sleep(3 * time.Second)
	}

	return nil
}

func checkDiskComponent() error {
	baseURL := fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort)
	componentName := "disk"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	states, err := clientv1.GetHealthStates(ctx, baseURL, clientv1.WithComponent(componentName))
	if err != nil {
		// assume disk component is enabled for all platforms
		return err
	}
	if len(states) == 0 {
		log.Logger.Warnw("empty state returned", "component", componentName)
		return errors.New("empty state returned")
	}

	for _, ss := range states {
		for _, s := range ss.States {
			log.Logger.Infof("state: %q, health: %s, extra info: %q\n", s.Name, s.Health, s.ExtraInfo)
		}
	}

	return nil
}

func checkNvidiaInfoComponent() error {
	baseURL := fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort)
	componentName := "accelerator-nvidia-info"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	states, err := clientv1.GetHealthStates(ctx, baseURL, clientv1.WithComponent(componentName))
	if err != nil {
		if errdefs.IsNotFound(err) {
			log.Logger.Warnw("component not found", "component", componentName)
			return nil
		}
		return err
	}
	if len(states) == 0 {
		log.Logger.Warnw("empty state returned", "component", componentName)
		return errors.New("empty state returned")
	}

	for _, ss := range states {
		for _, s := range ss.States {
			log.Logger.Infof("state: %q, health: %v, extra info: %q\n", s.Name, s.Health, s.ExtraInfo)
		}
	}

	return nil
}
