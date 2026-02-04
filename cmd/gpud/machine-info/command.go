package machineinfo

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/log"
	pkgmachineinfo "github.com/leptonai/gpud/pkg/machine-info"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/netutil"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.CreateLogger(zapLvl, ""))

	log.Logger.Debugw("starting machine-info command")

	stateFile, err := gpudcommon.StateFileFromContext(cliContext)
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}

	// only read the state file if it exists (existing gpud login)
	if _, err := os.Stat(stateFile); err == nil {
		dbRW, err := sqlite.Open(stateFile)
		if err != nil {
			return fmt.Errorf("failed to open state file: %w", err)
		}
		defer func() {
			_ = dbRW.Close()
		}()

		dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
		if err != nil {
			return fmt.Errorf("failed to open state file: %w", err)
		}
		defer func() {
			_ = dbRO.Close()
		}()

		rootCtx, rootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer rootCancel()
		machineID, err := pkgmetadata.ReadMachineID(rootCtx, dbRO)
		if err != nil {
			return err
		}

		fmt.Printf("GPUd machine ID: %q\n\n", machineID)
	}

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return err
	}

	machineInfo, err := pkgmachineinfo.GetMachineInfo(nvmlInstance)
	if err != nil {
		return err
	}
	machineInfo.RenderTable(os.Stdout)

	pubIP, _ := netutil.PublicIP()
	providerInfo := pkgmachineinfo.GetProvider(pubIP)
	if providerInfo == nil {
		fmt.Printf("%s failed to find provider (%v)\n", cmdcommon.WarningSign, err)
	} else {
		if providerInfo.PrivateIP == "" {
			if machineInfo != nil && machineInfo.NICInfo != nil {
				for _, iface := range machineInfo.NICInfo.PrivateIPInterfaces {
					if iface.IP == "" {
						continue
					}
					if iface.Addr.IsPrivate() && iface.Addr.Is4() {
						providerInfo.PrivateIP = iface.IP
						break
					}
				}
			}
		}
		fmt.Printf("%s successfully found provider %s\n", cmdcommon.CheckMark, providerInfo.Provider)
		providerInfo.RenderTable(os.Stdout)
	}

	return nil
}
