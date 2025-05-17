package customplugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"

	apiv1 "github.com/leptonai/gpud/api/v1"
	cmdcommon "github.com/leptonai/gpud/cmd/common"
	"github.com/leptonai/gpud/components"
	nvidiacommon "github.com/leptonai/gpud/pkg/config/common"
	customplugins "github.com/leptonai/gpud/pkg/custom-plugins"
	custompluginstestdata "github.com/leptonai/gpud/pkg/custom-plugins/testdata"
	"github.com/leptonai/gpud/pkg/log"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia-query/nvml"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	args := cliContext.Args()
	var specs customplugins.Specs
	if len(args) == 0 {
		log.Logger.Infow("using example specs")
		specs = custompluginstestdata.ExampleSpecs()
	} else {
		specs, err = customplugins.LoadSpecs(args[0])
		if err != nil {
			return err
		}
	}

	// execute "init" type plugins first
	sort.Slice(specs, func(i, j int) bool {
		// "init" type first
		if specs[i].Type == "init" && specs[j].Type == "init" {
			return i < j
		}
		return specs[i].Type == "init"
	})

	println()
	specs.PrintValidateResults(os.Stdout, cmdcommon.CheckMark, cmdcommon.WarningSign)
	println()

	if verr := specs.Validate(); verr != nil {
		return verr
	}

	customPluginsRun := cliContext.Bool("run")
	if !customPluginsRun {
		log.Logger.Infow("custom plugins are not run, only validating the specs")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	nvmlInstance, err := nvidianvml.New()
	if err != nil {
		return err
	}

	gpudInstance := &components.GPUdInstance{
		RootCtx:      ctx,
		NVMLInstance: nvmlInstance,
		NVIDIAToolOverwrites: nvidiacommon.ToolOverwrites{
			IbstatCommand:   cliContext.String("ibstat-command"),
			IbstatusCommand: cliContext.String("ibstatus-command"),
		},
	}

	customPluginsFailFast := cliContext.Bool("fail-fast")
	results, err := specs.ExecuteInOrder(gpudInstance, customPluginsFailFast)
	if err != nil {
		return err
	}

	println()
	for _, rs := range results {
		debugger, ok := rs.(components.CheckResultDebugger)
		if ok {
			fmt.Printf("\n### Component %q output\n\n%s\n\n", rs.ComponentName(), debugger.Debug())
		}
	}

	println()
	fmt.Printf("### Results\n\n")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	table.SetRowLine(true)
	table.SetAutoWrapText(false)
	table.SetHeader([]string{"Component", "Health State", "Summary", "Error", "Run Mode", "Extra Info"})
	for _, rs := range results {
		healthState := cmdcommon.CheckMark + " " + string(apiv1.HealthStateTypeHealthy)
		if rs.HealthStateType() != apiv1.HealthStateTypeHealthy {
			healthState = cmdcommon.WarningSign + " " + string(rs.HealthStateType())
		}

		err := ""
		runMode := ""
		extraInfo := ""

		states := rs.HealthStates()
		if len(states) > 0 {
			err = states[0].Error
			runMode = string(states[0].RunMode)

			b, _ := json.Marshal(states[0].ExtraInfo)
			extraInfo = string(b)
		}

		table.Append([]string{rs.ComponentName(), healthState, rs.Summary(), err, runMode, extraInfo})
	}
	table.Render()
	println()

	return nil
}
