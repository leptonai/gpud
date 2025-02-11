package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/urfave/cli"

	client "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/config"
	"github.com/leptonai/gpud/errdefs"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/systemd"
)

func cmdStatus(cliContext *cli.Context) error {
	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	if systemd.SystemctlExists() {
		active, err := systemd.IsActive("gpud.service")
		if err != nil {
			return err
		}
		if !active {
			fmt.Printf("%s gpud is not running\n", warningSign)
			return nil
		}
		fmt.Printf("%s gpud is running\n", checkMark)
	}
	fmt.Printf("%s successfully checked gpud status\n", checkMark)

	if err := checkDiskComponent(); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked whether disk component is running\n", checkMark)

	if err := checkNvidiaInfoComponent(); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked whether accelerator-nvidia-info component is running\n", checkMark)

	if err := client.BlockUntilServerReady(
		rootCtx,
		fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort),
	); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked gpud health\n", checkMark)

	for {
		cctx, ccancel := context.WithTimeout(rootCtx, 15*time.Second)
		packageStatus, err := client.GetPackageStatus(cctx, fmt.Sprintf("https://localhost:%d/admin/packages", config.DefaultGPUdPort))
		ccancel()
		if err != nil {
			fmt.Printf("%s failed to get package status: %v\n", warningSign, err)
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
	states, err := client.GetStates(ctx, baseURL, client.WithComponent(componentName))
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
			log.Logger.Infof("state: %q, healthy: %v, extra info: %q\n", s.Name, s.Healthy, s.ExtraInfo)
		}
	}

	return nil
}

func checkNvidiaInfoComponent() error {
	baseURL := fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort)
	componentName := "accelerator-nvidia-info"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	states, err := client.GetStates(ctx, baseURL, client.WithComponent(componentName))
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
			log.Logger.Infof("state: %q, healthy: %v, extra info: %q\n", s.Name, s.Healthy, s.ExtraInfo)
		}
	}

	return nil
}
