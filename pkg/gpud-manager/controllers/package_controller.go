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
			if version == pkg.TargetVersion {
				log.Logger.Debugf("[package controller]: %v version is %v (same as target, no-op)", pkg.Name, version)
				continue
			}

			log.Logger.Infof("[package controller]: %v version is %v, target is %v", pkg.Name, version, pkg.TargetVersion)
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
			err = runCommand(ctx, pkg.ScriptPath, "upgrade", nil)
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
		for _, pkg := range c.packageStatus {
			var skipCheck bool
			for _, dep := range pkg.Dependency {
				if _, ok := c.packageStatus[dep[0]]; !ok {
					log.Logger.Infof("[package controller]: %v dependency %v not found, skipping", pkg.Name, dep[0])
					skipCheck = true
					break
				}
				if !c.packageStatus[dep[0]].IsInstalled {
					log.Logger.Infof("[package controller]: %v dependency %v not installed, skipping", pkg.Name, dep[0])
					skipCheck = true
					break
				}
				if dep[1] != "*" && (c.packageStatus[dep[0]].CurrentVersion == "" || c.packageStatus[dep[0]].CurrentVersion < dep[1]) {
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
				err = runCommand(ctx, pkg.ScriptPath, "install", nil)
				close(done)
				if err != nil {
					log.Logger.Errorf("[package controller]: %v unexpected install failure: %v", pkg.Name, err)
				} else {
					if err = runCommand(ctx, pkg.ScriptPath, "start", nil); err != nil {
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
				c.Lock()
				c.packageStatus[pkg.Name].Status = true
				c.Unlock()
				log.Logger.Debugf("[package controller]: %v status ok", pkg.Name)
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

	finCh := make(chan struct{})
	if result != nil {
		go func() {
			defer close(finCh)
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
	var retErr error
	select {
	case <-ctx.Done():
		retErr = ctx.Err()
	case err = <-p.Wait():
		if err != nil {
			retErr = err
		}
	}
	if result != nil {
		<-finCh
	}
	return retErr
}
