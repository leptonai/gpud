package machineinfo

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli"

	apiv1 "github.com/leptonai/gpud/api/v1"
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
	outputFormat, err := gpudcommon.ParseOutputFormat(cliContext.String("output-format"))
	if err != nil {
		return err
	}
	wrapErr := func(code string, srcErr error) error {
		return gpudcommon.WrapOutputError(outputFormat, code, srcErr)
	}

	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return wrapErr("invalid_log_level", err)
	}
	if outputFormat == gpudcommon.OutputFormatJSON {
		log.SetLogger(nil)
	} else {
		log.SetLogger(log.CreateLogger(zapLvl, ""))
	}

	log.Logger.Debugw("starting machine-info command")

	stateFile, err := gpudcommon.StateFileFromContext(cliContext)
	if err != nil {
		return wrapErr("failed_to_get_state_file", fmt.Errorf("failed to get state file: %w", err))
	}

	machineID := ""

	// only read the state file if it exists (existing gpud login)
	if _, err := os.Stat(stateFile); err == nil {
		dbRW, err := sqlite.Open(stateFile)
		if err != nil {
			return wrapErr("failed_to_open_state_file", fmt.Errorf("failed to open state file: %w", err))
		}
		defer func() {
			_ = dbRW.Close()
		}()

		dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
		if err != nil {
			return wrapErr("failed_to_open_state_file", fmt.Errorf("failed to open state file: %w", err))
		}
		defer func() {
			_ = dbRO.Close()
		}()

		rootCtx, rootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer rootCancel()
		machineID, err = pkgmetadata.ReadMachineID(rootCtx, dbRO)
		if err != nil {
			return err
		}

		if outputFormat == gpudcommon.OutputFormatPlain {
			fmt.Printf("GPUd machine ID: %q\n\n", machineID)
		}
	}

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return wrapErr("failed_to_initialize_nvml", err)
	}

	machineInfo, err := pkgmachineinfo.GetMachineInfo(nvmlInstance)
	if err != nil {
		return wrapErr("failed_to_get_machine_info", err)
	}
	if outputFormat == gpudcommon.OutputFormatPlain {
		machineInfo.RenderTable(os.Stdout)
	}

	pubIP, _ := netutil.PublicIP()
	providerInfo := pkgmachineinfo.GetProvider(pubIP)
	if providerInfo == nil {
		if outputFormat == gpudcommon.OutputFormatPlain {
			fmt.Printf("%s failed to find provider (%v)\n", cmdcommon.WarningSign, err)
		}
	} else {
		var nicInfo *apiv1.MachineNICInfo
		if machineInfo != nil {
			nicInfo = machineInfo.NICInfo
		}
		providerInfo.PrivateIP = pkgmachineinfo.SelectPrivateIP(providerInfo.PrivateIP, providerInfo.PublicIP, nicInfo)
		// Match LoginRequest behavior: when provider metadata cannot provide
		// a usable region, use DERP/latency-derived machine location.
		if providerInfo.Region == "" {
			if loc := pkgmachineinfo.GetMachineLocation(); loc != nil && loc.Region != "" {
				providerInfo.Region = loc.Region
			}
		}
		if outputFormat == gpudcommon.OutputFormatPlain {
			fmt.Printf("%s successfully found provider %s\n", cmdcommon.CheckMark, providerInfo.Provider)
			providerInfo.RenderTable(os.Stdout)
		}
	}

	if outputFormat == gpudcommon.OutputFormatJSON {
		payload := map[string]any{
			"machine_id":   machineID,
			"machine_info": machineInfo,
			"provider":     providerInfo,
		}
		return wrapErr("failed_to_write_json_output", gpudcommon.WriteJSON(payload))
	}

	return nil
}
