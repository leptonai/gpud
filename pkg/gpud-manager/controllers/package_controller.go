package controllers

import (
	"context"
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

func (c *PackageController) setCurrentVersion(pkg string, version string) {
	c.Lock()
	defer c.Unlock()

	if c.packageStatus[pkg] == nil {
		return
	}

	c.packageStatus[pkg].CurrentVersion = version
}

func (c *PackageController) setProgress(pkg string, progress int) {
	c.Lock()
	defer c.Unlock()

	if c.packageStatus[pkg] == nil {
		return
	}

	c.packageStatus[pkg].Progress = progress
}

func (c *PackageController) setInstallingProgress(pkg string, installing bool, progress int) {
	c.Lock()
	defer c.Unlock()

	if c.packageStatus[pkg] == nil {
		return
	}

	c.packageStatus[pkg].Installing = installing
	c.packageStatus[pkg].Progress = progress
}

func (c *PackageController) setInstalledProgress(pkg string, installed bool, progress int) {
	c.Lock()
	defer c.Unlock()

	if c.packageStatus[pkg] == nil {
		return
	}

	c.packageStatus[pkg].IsInstalled = installed
	c.packageStatus[pkg].Progress = progress
}

func (c *PackageController) setStatus(pkg string, status bool) {
	c.Lock()
	defer c.Unlock()

	if c.packageStatus[pkg] == nil {
		return
	}

	c.packageStatus[pkg].Status = status
}

func (c *PackageController) getTotalTime(pkg string) time.Duration {
	c.RLock()
	defer c.RUnlock()

	if c.packageStatus[pkg] == nil {
		return 0
	}

	return c.packageStatus[pkg].TotalTime
}

// getPackageStatus returns the package status for the given package name.
// if the package is not found, it returns nil.
func (c *PackageController) getPackageStatus(pkg string) *packages.PackageStatus {
	c.RLock()
	defer c.RUnlock()

	return c.packageStatus[pkg]
}

func (c *PackageController) getIsInstalled(pkg string) bool {
	c.RLock()
	defer c.RUnlock()

	if c.packageStatus[pkg] == nil {
		return false
	}

	return c.packageStatus[pkg].IsInstalled
}

func (c *PackageController) getCurrentVersion(pkg string) string {
	c.RLock()
	defer c.RUnlock()

	if c.packageStatus[pkg] == nil {
		return ""
	}

	return c.packageStatus[pkg].CurrentVersion
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
		for _, pkg := range c.packageStatus {
			if !pkg.IsInstalled {
				continue
			}
			var version string
			err := runCommand(ctx, pkg.ScriptPath, "version", &version)
			if err != nil || version == "" {
				log.Logger.Errorf("[package controller]: %v unexpected version failure: %v, version: %s", pkg.Name, err, version)
				continue
			}
			log.Logger.Infof("[package controller]: %v version is %v, target is %v", pkg.Name, version, pkg.TargetVersion)
			c.setCurrentVersion(pkg.Name, version)
			if version == pkg.TargetVersion {
				continue
			}
			eta := c.getTotalTime(pkg.Name)
			c.setInstallingProgress(pkg.Name, true, 0)
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
						c.setProgress(pkg.Name, progress)
					}
				}
			}()
			err = runCommand(ctx, pkg.ScriptPath, "upgrade", nil)
			close(done)
			c.setInstallingProgress(pkg.Name, false, 100)
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
		for _, pkg := range c.packageStatus {
			var skipCheck bool
			for _, dep := range pkg.Dependency {
				depStatus := c.getPackageStatus(dep[0])
				if depStatus == nil {
					log.Logger.Infof("[package controller]: %v dependency %v not found, skipping", pkg.Name, dep[0])
					skipCheck = true
					break
				}

				isInstalled := c.getIsInstalled(dep[0])
				if !isInstalled {
					log.Logger.Infof("[package controller]: %v dependency %v not installed, skipping", pkg.Name, dep[0])
					skipCheck = true
					break
				}

				curVer := c.getCurrentVersion(dep[0])
				if curVer == "" || curVer < dep[1] {
					log.Logger.Infof("[package controller]: %v dependency %v version %v does not meet required %v, skipping", pkg.Name, dep[0], c.packageStatus[dep[0]].CurrentVersion, dep[1])
					skipCheck = true
					break
				}
			}
			if skipCheck {
				continue
			}
			if pkg.Installing {
				log.Logger.Infof("[package controller]: %v installing...", pkg.Name)
				continue
			}
			// if installing, then skip
			err := runCommand(ctx, pkg.ScriptPath, "isInstalled", nil)
			if err == nil {
				c.setInstalledProgress(pkg.Name, true, 100)
				log.Logger.Infof("[package controller]: %v already installed", pkg.Name)
				continue
			}
			log.Logger.Errorf("[package controller]: %v not installed, installing", pkg.Name)
			go func() {
				eta := c.getTotalTime(pkg.Name)
				c.setInstallingProgress(pkg.Name, true, 0)
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
							c.setProgress(pkg.Name, progress)
						}
					}
				}()
				err = runCommand(ctx, pkg.ScriptPath, "install", nil)
				close(done)
				if err != nil {
					log.Logger.Errorf("[package controller]: %v unexpected install failure: %v", pkg.Name, err)
				} else {
					if err = runCommand(ctx, pkg.ScriptPath, "start", nil); err != nil {
						log.Logger.Errorf("[package controller]: %v failed to start after installing: %v", pkg.Name, err)
					}
				}
				c.setInstallingProgress(pkg.Name, false, 100)
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
		for _, pkg := range c.packageStatus {
			if err := runCommand(ctx, pkg.ScriptPath, "needDelete", nil); err != nil {
				continue
			}
			err := runCommand(ctx, pkg.ScriptPath, "delete", nil)
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
		for _, pkg := range c.packageStatus {
			if !pkg.IsInstalled {
				continue
			}
			err := runCommand(ctx, pkg.ScriptPath, "status", nil)
			if err == nil {
				c.setStatus(pkg.Name, true)
				log.Logger.Infof("[package controller]: %v status ok", pkg.Name)
				continue
			}
			log.Logger.Errorf("[package controller]: %v status not ok, restarting", pkg.Name)
			if err = runCommand(ctx, pkg.ScriptPath, "stop", nil); err != nil {
				log.Logger.Errorf("[package controller]: %v unexpected stop failure: %v", pkg.Name, err)
				continue
			}
			if err = runCommand(ctx, pkg.ScriptPath, "start", nil); err != nil {
				log.Logger.Errorf("[package controller]: %v unexpected start failure: %v", pkg.Name, err)
			}
		}
	}
}

func runCommand(ctx context.Context, script, arg string, result *string) error {
	var ops []process.OpOption
	if result == nil {
		f, err := os.OpenFile(filepath.Join(filepath.Dir(script), arg+".log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
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
		go func() {
			lines := make([]string, 0)
			err := process.Read(
				ctx,
				p,
				// only read stdout to check the version output
				process.WithReadStdout(),
				process.WithProcessLine(func(line string) {
					lines = append(lines, line)
				}),
			)
			output := strings.Join(lines, "\n")
			if err == nil {
				*result = output
			} else {
				*result = fmt.Sprintf("failed to run '%s %s' with error %v\n\noutput:\n%s", script, arg, err, output)
			}
		}()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err = <-p.Wait():
		if err != nil {
			return err
		}
	}
	return nil
}
