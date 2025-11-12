// Package up implements the "up" command.
package up

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli"

	cmdlogin "github.com/leptonai/gpud/cmd/gpud/login"
	"github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/osutil"
	pkdsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
	pkgvalidation "github.com/leptonai/gpud/pkg/validation"
)

func Command(cliContext *cli.Context) (retErr error) {
	logLevel := cliContext.String("log-level")
	logFile := cliContext.String("log-file")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, logFile)

	log.Logger.Debugw("starting up command")

	if err := osutil.RequireRoot(); err != nil {
		return err
	}

	token := cliContext.String("token")

	if token != "" {
		log.Logger.Debugw("token provided, validating minimum platform resources")

		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()

		reqs, err := pkgvalidation.GetPlatformRequirements(cctx)
		if err != nil {
			log.Logger.Warnw("failed to fetch platform resource information to evaluate minimum resource requirements", "error", err)
		} else if err := reqs.Check(); err != nil {
			log.Logger.Warnw("minimum resource requirements not satisfied",
				"observed_logical_cpu_cores", reqs.LogicalCPUCores,
				"minimum_logical_cpu_cores", reqs.MinimumCPUCores,
				"observed_memory", reqs.FormatMemoryHumanized(),
				"minimum_memory", reqs.FormatMinimumMemoryHumanized(),
			)

			shouldContinue, promptErr := promptMinimumRequirementOverride(reqs)
			if promptErr != nil {
				return promptErr
			}
			if !shouldContinue {
				return errors.New("aborted by user due to insufficient platform resources")
			}
		} else {
			log.Logger.Debugw("minimum resource requirements satisfied")
		}
	}

	// step 1.
	// perform "login" if and only if configured
	if token != "" {
		log.Logger.Debugw("non-empty --token provided, logging in")
		if lerr := cmdlogin.Command(cliContext); lerr != nil {
			return lerr
		}
		log.Logger.Debugw("successfully logged in")
	} else {
		log.Logger.Infow("no --token provided, skipping login")
	}

	// step 2.
	// perform "run" to start the daemon in systemd service
	bin, err := os.Executable()
	if err != nil {
		return err
	}

	log.Logger.Debugw("checking if systemd exists")
	if !pkdsystemd.SystemctlExists() {
		return fmt.Errorf("requires systemd, to run without systemd, '%s run'", bin)
	}
	log.Logger.Debugw("systemd exists")

	log.Logger.Debugw("checking if gpud binary exists")
	if !systemd.DefaultBinExists() {
		return fmt.Errorf("gpud binary not found at %s (you may run 'cp %s %s' to fix the installation)", systemd.DefaultBinPath, bin, systemd.DefaultBinPath)
	}
	log.Logger.Debugw("gpud binary exists")

	log.Logger.Debugw("starting systemd init")
	endpoint := cliContext.String("endpoint")
	if err := systemdInit(endpoint); err != nil {
		return err
	}
	log.Logger.Debugw("successfully started systemd init")

	log.Logger.Debugw("enabling systemd unit")
	if err := pkgupdate.EnableGPUdSystemdUnit(); err != nil {
		return err
	}
	log.Logger.Debugw("successfully enabled systemd unit")

	log.Logger.Debugw("restarting systemd unit")
	if err := pkgupdate.RestartGPUdSystemdUnit(); err != nil {
		return err
	}
	log.Logger.Debugw("successfully restarted systemd unit")

	log.Logger.Debugw("successfully started gpud (run 'gpud status' for checking status)")
	return nil
}

func systemdInit(endpoint string) error {
	if err := systemd.CreateDefaultEnvFile(endpoint); err != nil {
		return err
	}
	systemdUnitFileData := systemd.GPUdServiceUnitFileContents()
	return os.WriteFile(systemd.DefaultUnitFile, []byte(systemdUnitFileData), 0644)
}

func promptMinimumRequirementOverride(requirements pkgvalidation.PlatformRequirements) (bool, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf(`
This host currently reports %d logical CPU cores and %s of memory.
Joining it to the Lepton platform installs kubelet and other system components that expect at least %d logical CPU cores and %s of memory.
Continuing with fewer resources is likely to leave kubelet stuck restarting.
Do you want to continue anyway? [y/N]: `,
		requirements.LogicalCPUCores,
		requirements.FormatMemoryHumanized(),
		requirements.MinimumCPUCores,
		requirements.FormatMinimumMemoryHumanized(),
	)

	response, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("failed to read confirmation input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "y" || response == "yes" {
		fmt.Println("continuing despite not meeting the minimum platform requirements")
		return true, nil
	}

	fmt.Println("aborting gpud up; please ensure the host satisfies the minimum requirements and retry")
	return false, nil
}
