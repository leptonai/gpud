package down

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/urfave/cli"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/config"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/osutil"
	"github.com/leptonai/gpud/pkg/sqlite"
	pkgsystemd "github.com/leptonai/gpud/pkg/systemd"
	pkgupdate "github.com/leptonai/gpud/pkg/update"
)

func Command(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting down command")

	if err := osutil.RequireRoot(); err != nil {
		return err
	}

	if !pkgsystemd.SystemctlExists() {
		fmt.Printf("%s requires systemd, if not run by systemd, manually kill the process with 'pidof gpud'\n", cmdcommon.WarningSign)
		os.Exit(1)
	}

	active, err := pkgsystemd.IsActive("gpud.service")
	if err != nil {
		fmt.Printf("%s failed to check if gpud is running: %v\n", cmdcommon.WarningSign, err)
		os.Exit(1)
	}
	if !active {
		fmt.Printf("%s gpud is not running (no-op)\n", cmdcommon.CheckMark)
		os.Exit(0)
	}

	if err := pkgupdate.StopSystemdUnit(); err != nil {
		fmt.Printf("%s failed to stop systemd unit 'gpud.service': %v\n", cmdcommon.WarningSign, err)
		os.Exit(1)
	}

	if err := pkgupdate.DisableGPUdSystemdUnit(); err != nil {
		fmt.Printf("%s failed to disable systemd unit 'gpud.service': %v\n", cmdcommon.WarningSign, err)
		os.Exit(1)
	}

	fmt.Printf("%s successfully stopped gpud\n", cmdcommon.CheckMark)

	dataDir, err := gpudcommon.ResolveDataDir(cliContext)
	if err != nil {
		return fmt.Errorf("failed to get data dir: %w", err)
	}

	if cliContext.Bool("cleanup-packages") {
		// NOTE: This only cleans packages that already have "needDelete" markers.
		// These markers are created by the control plane via "gpud_init.sh stop" when
		// the machine enters the deleting stage. If markers don't exist, cleanup is skipped.
		//
		// This means: if the session disconnected BEFORE gpud_init.sh stop ran,
		// orphaned packages won't be cleaned. This is intentional to avoid accidentally
		// cleaning packages on machines that aren't actually being deleted.
		//
		// TODO: Add --force-cleanup flag to create markers and run delete for orphan recovery.
		// This would allow operators to manually trigger cleanup on machines with orphaned state.
		fmt.Printf("%s cleaning up packages (only those marked for deletion)...\n", cmdcommon.CheckMark)
		if err := cleanupPackages(dataDir); err != nil {
			fmt.Printf("%s package cleanup had errors: %v\n", cmdcommon.WarningSign, err)
		} else {
			fmt.Printf("%s package cleanup completed\n", cmdcommon.CheckMark)
		}
	}

	if cliContext.Bool("reset-state") {
		log.Logger.Warnw("resetting state")

		stateFile := config.StateFilePath(dataDir)
		log.Logger.Debugw("opening state file for writing", "file", stateFile)

		dbRW, err := sqlite.Open(stateFile)
		if err != nil {
			return fmt.Errorf("failed to open state file %q: %w", stateFile, err)
		}
		defer func() {
			_ = dbRW.Close()
		}()

		rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer rootCancel()

		if err := pkgmetadata.DeleteAllMetadata(rootCtx, dbRW); err != nil {
			return fmt.Errorf("failed to delete metadata: %w", err)
		}
		fmt.Printf("%s successfully reset state\n", cmdcommon.CheckMark)
	}

	return nil
}

// cleanupPackages runs delete for packages that are already marked for deletion.
//
// Control plane assumption (as of today):
//   - Empty file    = pending delete (created by gpud_init.sh stop)
//   - Non-empty     = "Finished" (written by package script after successful delete)
//   - Missing file  = not marked for deletion
//
// This function does NOT create needDelete markers. It only processes packages
// that the control plane has already marked via gpud_init.sh stop.
//
// TODO: Add force parameter to create markers for orphan recovery scenarios.
func cleanupPackages(dataDir string) error {
	pkgDir := config.PackagesDir(dataDir)
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		log.Logger.Debugw("no packages directory", "path", pkgDir)
		return nil
	}

	// Collect packages that have needDelete markers (empty file = needs deletion)
	var packages []string
	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || path == pkgDir {
			return nil
		}

		scriptPath := filepath.Join(path, "init.sh")
		if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
			return filepath.SkipDir // no init.sh, not a package
		}

		// Control plane assumption (as of today):
		// - Empty file   = pending delete
		// - Non-empty    = "Finished"
		// - Missing      = not marked
		needDeletePath := filepath.Join(path, "needDelete")
		info, err := os.Stat(needDeletePath)
		if err != nil {
			// Case: Missing file → not marked for deletion, skip
			return filepath.SkipDir
		}
		if info.Size() > 0 {
			// Case: Non-empty file → already finished, skip
			return filepath.SkipDir
		}

		// Case: Empty file (Size == 0) → pending delete, add to list
		packages = append(packages, path)
		return filepath.SkipDir
	})
	if err != nil {
		return fmt.Errorf("failed to scan packages: %w", err)
	}

	if len(packages) == 0 {
		log.Logger.Infow("no packages marked for deletion")
		return nil
	}

	log.Logger.Infow("found packages marked for deletion", "count", len(packages))

	// Run delete multiple times to handle dependency chains.
	// Example: kubelet depends on containerd. On first pass:
	//   - kubelet delete fails (containerd still running)
	//   - containerd delete succeeds
	// On second pass:
	//   - kubelet delete succeeds (containerd now gone)
	const maxIterations = 3
	var lastErrs []error

	for iter := 1; iter <= maxIterations; iter++ {
		var pending []string
		var iterErrs []error

		for _, pkgPath := range packages {
			pkgName := filepath.Base(pkgPath)
			scriptPath := filepath.Join(pkgPath, "init.sh")

			// Re-check if package still needs deletion based on marker state:
			// - Missing file (err != nil) → not marked, skip
			// - Non-empty file (Size > 0) → already finished, skip
			// - Empty file (Size == 0) → pending delete, proceed
			needDeletePath := filepath.Join(pkgPath, "needDelete")
			info, err := os.Stat(needDeletePath)
			if err != nil || info.Size() > 0 {
				continue // missing or finished, skip
			}

			// Run delete
			cmd := exec.Command("bash", scriptPath, "delete")
			cmd.Dir = pkgPath
			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Logger.Debugw("package delete failed", "package", pkgName, "iteration", iter, "error", err)
				iterErrs = append(iterErrs, fmt.Errorf("%s: %w", pkgName, err))
				pending = append(pending, pkgPath)
			} else {
				log.Logger.Infow("package delete succeeded", "package", pkgName, "iteration", iter)
			}
			_ = output // logged at debug level if needed
		}

		packages = pending
		lastErrs = iterErrs

		if len(pending) == 0 {
			break
		}
		log.Logger.Infow("retrying failed packages", "remaining", len(pending), "iteration", iter)
	}

	if len(lastErrs) > 0 {
		return fmt.Errorf("%d package(s) failed cleanup after %d iterations", len(lastErrs), maxIterations)
	}
	return nil
}
