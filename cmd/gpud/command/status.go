package command

import (
	"context"
	"fmt"
	"time"

	client "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/config"
	"github.com/leptonai/gpud/manager"
	"github.com/leptonai/gpud/pkg/systemd"

	"github.com/urfave/cli"
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

	if err := client.BlockUntilServerReady(
		rootCtx,
		fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort),
	); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked gpud health\n", checkMark)

	if manager.GlobalController != nil {
		fmt.Printf("%s package manager initialized\n", checkMark)
		packageStatus, err := manager.GlobalController.Status(rootCtx)
		if err != nil {
			fmt.Printf("%s failed to get status: %v\n", warningSign, err)
			return err
		}
		for _, status := range packageStatus {
			statusSign := warningSign
			if status.Status {
				statusSign = checkMark
			}
			fmt.Printf("%s %v version: %v target version: %v, status: %v installed: %v\n", statusSign, status.Name, status.CurrentVersion, status.TargetVersion, status.Status, status.IsInstalled)
		}
	}

	return nil
}
