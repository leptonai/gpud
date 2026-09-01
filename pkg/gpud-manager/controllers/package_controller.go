package controllers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/process"
)

type PackageController struct {
	fileWatcher   chan packages.PackageInfo
	packageStatus map[string]*packages.PackageStatus
	syncPeriod    time.Duration
	sync.RWMutex
}

func NewPackageController(watcher chan packages.PackageInfo) *PackageController {
	r := &PackageController{
		fileWatcher:   watcher,
		packageStatus: make(map[string]*packages.PackageStatus),
		syncPeriod:    3 * time.Second,
	}
	return r
}

func (c *PackageController) Status(ctx context.Context) ([]packages.PackageStatus, error) {
	c.RLock()
	defer c.RUnlock()
	var ret []packages.PackageStatus
	for _, pkg := range c.packageStatus {
		ret = append(ret, *pkg)
	}
	sort.Sort(packages.PackageStatuses(ret))
	return ret, nil
}

func (c *PackageController) Run(ctx context.Context) error {
	go c.reconcileLoop(ctx)
	go c.updateRunner(ctx)
	go c.installRunner(ctx)
	go c.statusRunner(ctx)
	go c.deleteRunner(ctx)
	return nil
}

func (c *PackageController) reconcileLoop(ctx context.Context) {
	for {
		select {
		case packageInfo := <-c.fileWatcher:
			c.Lock()
			log.Logger.Infof("[package controller]: received package info: %v", packageInfo)
			if _, ok := c.packageStatus[packageInfo.Name]; !ok {
				c.packageStatus[packageInfo.Name] = &packages.PackageStatus{
					Name:           packageInfo.Name,
					Skipped:        false,
					IsInstalled:    false,
					Installing:     false,
					Progress:       0,
					Status:         false,
					TargetVersion:  "",
					CurrentVersion: "",
					ScriptPath:     "",
					Dependency:     packageInfo.Dependency,
					TotalTime:      packageInfo.TotalTime,
				}
			}
			c.packageStatus[packageInfo.Name].TotalTime = packageInfo.TotalTime
			c.packageStatus[packageInfo.Name].Dependency = packageInfo.Dependency
			c.packageStatus[packageInfo.Name].TargetVersion = packageInfo.TargetVersion
			c.packageStatus[packageInfo.Name].ScriptPath = packageInfo.ScriptPath
			c.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// packageStatusSnapshot returns a point-in-time copy of the package map under
// the read lock. Runners iterate the snapshot so they never traverse the map
// concurrently with reconcileLoop, which inserts entries under the write lock.
func (c *PackageController) packageStatusSnapshot() []*packages.PackageStatus {
	c.RLock()
	defer c.RUnlock()
	pkgs := make([]*packages.PackageStatus, 0, len(c.packageStatus))
	for _, pkg := range c.packageStatus {
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func (c *PackageController) updateRunner(ctx context.Context) {
	ticker := time.NewTicker(c.syncPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticker.Reset(c.syncPeriod)
		}
		for _, pkg := range c.packageStatusSnapshot() {
			c.RLock()
			installed := pkg.IsInstalled
			scriptPath := pkg.ScriptPath
			targetVersion := pkg.TargetVersion
			c.RUnlock()
			if !installed {
				continue
			}
			var version string
			err := runCommand(ctx, scriptPath, "version", &version)
			if err != nil {
				log.Logger.Errorf("[package controller]: %v unexpected version failure: %v", pkg.Name, err)
				continue
			}
			if version == "" {
				continue
			}
			c.Lock()
			c.packageStatus[pkg.Name].CurrentVersion = version
			c.Unlock()

			var shouldSkipResult string
			if err = runCommand(ctx, scriptPath, "shouldSkip", &shouldSkipResult); err == nil {
				c.Lock()
				c.packageStatus[pkg.Name].Skipped = true
				c.Unlock()
				log.Logger.Debugf("[package controller]: %v shouldSkip returned 0, skipping update", pkg.Name)
				continue
			}

			if version == targetVersion {
				log.Logger.Debugf("[package controller]: %v version is %v (same as target, no-op)", pkg.Name, version)
				continue
			}

			log.Logger.Infof("[package controller]: %v version is %v, target is %v", pkg.Name, version, targetVersion)
			var eta time.Duration
			c.Lock()
			c.packageStatus[pkg.Name].Installing = true
			c.packageStatus[pkg.Name].Progress = 0
			eta = c.packageStatus[pkg.Name].TotalTime
			c.Unlock()
			done := make(chan any)
			go func() {
				startTime := time.Now()
				localTicker := time.NewTicker(2 * time.Second)
				defer localTicker.Stop()
				for {
					select {
					case <-done:
						return
					case <-localTicker.C:
						c.Lock()
						progress := int(time.Since(startTime).Seconds() / eta.Seconds() * 100)
						if progress >= 100 {
							progress = 98
						}
						c.packageStatus[pkg.Name].Progress = progress
						c.Unlock()
					}
				}
			}()
			err = runCommand(ctx, scriptPath, "upgrade", nil)
			close(done)
			c.Lock()
			c.packageStatus[pkg.Name].Installing = false
			c.packageStatus[pkg.Name].Progress = 100
			c.Unlock()
			if err != nil {
				log.Logger.Errorf("[package controller]: %v unexpected upgrade failure: %v", pkg.Name, err)
			}
		}
	}
}

func (c *PackageController) installRunner(ctx context.Context) {
	ticker := time.NewTicker(c.syncPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticker.Reset(c.syncPeriod)
		}
		for _, pkg := range c.packageStatusSnapshot() {
			c.RLock()
			dependency := pkg.Dependency
			scriptPath := pkg.ScriptPath
			c.RUnlock()
			var skipCheck bool
			for _, dep := range dependency {
				// Read the dependency's mutable fields under the read lock;
				// updateRunner and the async install goroutine mutate them
				// under the write lock.
				c.RLock()
				depPkg, depFound := c.packageStatus[dep[0]]
				depInstalled := depFound && depPkg.IsInstalled
				depVersion := ""
				if depFound {
					depVersion = depPkg.CurrentVersion
				}
				c.RUnlock()
				if !depFound {
					log.Logger.Infof("[package controller]: %v dependency %v not found, skipping", pkg.Name, dep[0])
					skipCheck = true
					break
				}
				if !depInstalled {
					log.Logger.Infof("[package controller]: %v dependency %v not installed, skipping", pkg.Name, dep[0])
					skipCheck = true
					break
				}
				if dep[1] != "*" && (depVersion == "" || depVersion < dep[1]) {
					log.Logger.Infof("[package controller]: %v dependency %v version %v does not meet required %v, skipping", pkg.Name, dep[0], depVersion, dep[1])
					skipCheck = true
					break
				}
			}
			if skipCheck {
				continue
			}

			var shouldSkipResult string
			if err := runCommand(ctx, scriptPath, "shouldSkip", &shouldSkipResult); err == nil {
				c.Lock()
				c.packageStatus[pkg.Name].Skipped = true
				c.packageStatus[pkg.Name].Progress = 100
				c.packageStatus[pkg.Name].IsInstalled = true
				c.Unlock()
				log.Logger.Debugf("[package controller]: %v shouldSkip returned 0, skipping install", pkg.Name)
				continue
			}

			c.RLock()
			installing := pkg.Installing
			c.RUnlock()
			if installing {
				log.Logger.Infof("[package controller]: %v installing...", pkg.Name)
				continue
			}

			// if installing, then skip
			err := runCommand(ctx, scriptPath, "isInstalled", nil)
			if err == nil {
				c.Lock()
				c.packageStatus[pkg.Name].Progress = 100
				c.packageStatus[pkg.Name].IsInstalled = true
				c.Unlock()
				log.Logger.Debugw("[package controller] already installed", "name", pkg.Name)
				continue
			}

			log.Logger.Warnw("[package controller] not installed, installing", "name", pkg.Name, "error", err)
			go func() {
				var eta time.Duration
				c.Lock()
				c.packageStatus[pkg.Name].Installing = true
				c.packageStatus[pkg.Name].Progress = 0
				eta = c.packageStatus[pkg.Name].TotalTime
				c.Unlock()
				done := make(chan any)
				go func() {
					startTime := time.Now()
					localTicker := time.NewTicker(2 * time.Second)
					defer localTicker.Stop()
					for {
						select {
						case <-done:
							return
						case <-localTicker.C:
							progress := int(time.Since(startTime).Seconds() / eta.Seconds() * 100)
							if progress >= 100 {
								progress = 98
							}
							c.Lock()
							c.packageStatus[pkg.Name].Progress = progress
							c.Unlock()
						}
					}
				}()
				err = runCommand(ctx, scriptPath, "install", nil)
				close(done)
				if err != nil {
					log.Logger.Errorf("[package controller]: %v unexpected install failure: %v", pkg.Name, err)
				} else {
					if err = runCommand(ctx, scriptPath, "start", nil); err != nil {
						log.Logger.Errorf("[package controller]: %v failed to start after installing: %v", pkg.Name, err)
					}
				}
				c.Lock()
				c.packageStatus[pkg.Name].Installing = false
				c.packageStatus[pkg.Name].Progress = 100
				c.Unlock()
			}()
		}
	}
}

func (c *PackageController) deleteRunner(ctx context.Context) {
	ticker := time.NewTicker(c.syncPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticker.Reset(c.syncPeriod)
		}
		for _, pkg := range c.packageStatusSnapshot() {
			c.RLock()
			scriptPath := pkg.ScriptPath
			c.RUnlock()
			if err := runCommand(ctx, scriptPath, "needDelete", nil); err != nil {
				continue
			}
			err := runCommand(ctx, scriptPath, "delete", nil)
			if err != nil {
				log.Logger.Infof("[package controller]: %v failed to delete: %v", pkg.Name, err)
			}
		}
	}
}

func (c *PackageController) statusRunner(ctx context.Context) {
	ticker := time.NewTicker(c.syncPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ticker.Reset(c.syncPeriod)
		}
		for _, pkg := range c.packageStatusSnapshot() {
			c.RLock()
			installed := pkg.IsInstalled
			scriptPath := pkg.ScriptPath
			c.RUnlock()
			if !installed {
				continue
			}

			var shouldSkipResult string
			if err := runCommand(ctx, scriptPath, "shouldSkip", &shouldSkipResult); err == nil {
				c.Lock()
				c.packageStatus[pkg.Name].Skipped = true
				c.packageStatus[pkg.Name].Status = true
				c.Unlock()
				log.Logger.Debugf("[package controller]: %v shouldSkip returned 0, setting status to true", pkg.Name)
				continue
			}

			err := runCommand(ctx, scriptPath, "status", nil)
			if err == nil {
				c.Lock()
				c.packageStatus[pkg.Name].Status = true
				c.Unlock()
				log.Logger.Debugf("[package controller]: %v status ok", pkg.Name)
				continue
			}
			log.Logger.Errorf("[package controller]: %v status not ok, restarting", pkg.Name)
			// Use the scriptPath snapshot taken under the read lock above:
			// reconcileLoop can update pkg.ScriptPath concurrently under the
			// write lock, so reading the field here without the lock is a
			// data race.
			if err = runCommand(ctx, scriptPath, "stop", nil); err != nil {
				log.Logger.Errorf("[package controller]: %v unexpected stop failure: %v", pkg.Name, err)
				continue
			}
			if err = runCommand(ctx, scriptPath, "start", nil); err != nil {
				log.Logger.Errorf("[package controller]: %v unexpected start failure: %v", pkg.Name, err)
			}
		}
	}
}

func runCommand(ctx context.Context, script, arg string, result *string) error {
	var ops []process.OpOption

	// WithAllowDetachedProcess(true) allows backgrounded commands (using "&")
	// to continue running after the script exits. This is critical for package
	// installation scripts that use patterns like:
	//   sleep 10 && systemctl restart gpud &
	//
	// Without WithAllowDetachedProcess(true):
	// - The "&" does not take effect - backgrounded processes are killed on Close()
	// - Package init.sh waits 10s and restarts instead of returning instantly
	// - Package controller thinks installation is blocking
	// - This causes delayed package installations and repeated restart loops
	ops = append(ops, process.WithAllowDetachedProcess(true))

	if result == nil {
		f, err := os.OpenFile(filepath.Join(filepath.Dir(script), arg+".log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := f.Close(); cerr != nil && !errors.Is(cerr, os.ErrClosed) {
				log.Logger.Warnw("failed to close log file", "error", cerr)
			}
		}()
		ops = append(ops, process.WithOutputFile(f))
	}

	p, err := process.New(append(ops, process.WithCommand("bash", script, arg))...)
	if err != nil {
		return err
	}
	if err = p.Start(ctx); err != nil {
		return err
	}
	defer func() {
		if err := p.Close(ctx); err != nil {
			log.Logger.Warnw("failed to abort command", "err", err)
		}
	}()

	if result != nil {
		// Read stdout in the current goroutine (not a separate goroutine)
		// to avoid a race where cmd.Wait() — called by the watchCmd goroutine
		// started in p.Start() — closes the StdoutPipe before a reader goroutine
		// has had a chance to drain it, resulting in empty or truncated output.
		// Reading inline ensures the scanner is blocked on the pipe read before
		// the child process even writes anything, making the race practically
		// impossible.
		lines := make([]string, 0)
		readErr := process.Read(
			ctx,
			p,
			// only read stdout to check the version output
			process.WithReadStdout(),
			process.WithProcessLine(func(line string) {
				lines = append(lines, line)
			}),
		)
		output := strings.Join(lines, "\n")
		if readErr == nil {
			*result = output
		} else {
			*result = fmt.Sprintf("failed to run '%s %s' with error %v\n\noutput:\n%s", script, arg, readErr, output)
		}
	}

	// Wait for the process to exit.  When result != nil the stdout pipe has
	// already been fully drained by process.Read above, so cmd.Wait() closing
	// it here is safe.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-p.Wait():
		return err
	}
}
