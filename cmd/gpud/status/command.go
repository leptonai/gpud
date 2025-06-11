package status

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/urfave/cli"

	clientv1 "github.com/leptonai/gpud/client/v1"
	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/process"
	"github.com/leptonai/gpud/pkg/server"
	"github.com/leptonai/gpud/pkg/sqlite"
	"github.com/leptonai/gpud/pkg/systemd"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting status command")

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	log.Logger.Debugw("getting state file")
	stateFile, err := config.DefaultStateFile()
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}
	log.Logger.Debugw("successfully got state file")

	log.Logger.Debugw("opening state file for reading")
	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()
	log.Logger.Debugw("successfully opened state file for reading")

	log.Logger.Debugw("reading machine id")
	machineID, err := pkgmetadata.ReadMetadata(rootCtx, dbRO, pkgmetadata.MetadataKeyMachineID)
	if err != nil {
		return fmt.Errorf("failed to read machine id: %w", err)
	}
	log.Logger.Debugw("successfully read machine id")

	log.Logger.Debugw("reading login success")
	loginSuccess, err := pkgmetadata.ReadMetadata(rootCtx, dbRO, pkgmetadata.MetadataKeyControlPlaneLoginSuccess)
	if err != nil {
		return fmt.Errorf("failed to read login success: %w", err)
	}
	log.Logger.Debugw("successfully read login success")

	if loginSuccess == "" {
		ts, err := strconv.ParseInt(loginSuccess, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse login success: %w", err)
		}
		loginTimeUTC := time.Unix(ts, 0)
		nowUTC := time.Now().UTC()
		loginTimeHumanized := humanize.RelTime(loginTimeUTC, nowUTC, "ago", "from now")
		fmt.Printf("%s login success at %s (machine id: %s)\n", cmdcommon.CheckMark, loginTimeHumanized, machineID)
	} else {
		fmt.Printf("%s login information not found\n", cmdcommon.CheckMark)
	}

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
