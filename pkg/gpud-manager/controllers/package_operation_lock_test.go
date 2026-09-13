// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leptonai/gpud/pkg/gpud-manager/packages"
	"github.com/leptonai/gpud/pkg/kapmtls"
	"github.com/leptonai/gpud/pkg/packagelock"
)

func TestPackageOperationsSkipBusyPackage(t *testing.T) {
	mockey.PatchConvey("a rotation excludes every lifecycle operation without blocking other packages", t, func() {
		c := NewPackageController(nil)
		pkg := &packages.PackageStatus{Name: kapmtls.PackageName, IsInstalled: true}
		other := &packages.PackageStatus{Name: t.Name(), IsInstalled: true}
		c.packageStatus[pkg.Name] = pkg
		c.packageStatus[other.Name] = other
		var calls int
		mockey.Mock(runCommand).To(func(context.Context, string, string, *string) error {
			calls++
			return nil
		}).Build()
		mu := packagelock.For(kapmtls.PackageName)
		mu.Lock()
		defer mu.Unlock()
		for _, operation := range []func(context.Context, *packages.PackageStatus){
			c.statusPackage, c.installPackage, c.updatePackage, c.deletePackage,
		} {
			operation(context.Background(), pkg)
		}
		assert.Zero(t, calls)
		c.statusPackage(context.Background(), other)
		assert.Positive(t, calls)
	})
}

func TestPackageStatusRecoveryIsAtomic(t *testing.T) {
	for _, failedCommand := range []string{"", "stop", "start"} {
		t.Run("failure-"+failedCommand, func(t *testing.T) {
			mockey.PatchConvey("status and recovery share one operation lock, including errors", t, func() {
				c := NewPackageController(nil)
				pkg := &packages.PackageStatus{Name: t.Name(), IsInstalled: true}
				c.packageStatus[pkg.Name] = pkg
				mu := packagelock.For(pkg.Name)
				var calls []string
				mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
					calls = append(calls, arg)
					if mu.TryLock() {
						mu.Unlock()
						t.Error("operation mutex released before recovery completed")
					}
					// Commands must not run with the controller state mutex held.
					_, err := c.Status(ctx)
					assert.NoError(t, err)
					if arg == "shouldSkip" || arg == "status" || arg == failedCommand {
						return errors.New("command failed")
					}
					return nil
				}).Build()
				c.statusPackage(context.Background(), pkg)
				want := []string{"shouldSkip", "status", "stop", "start"}
				if failedCommand == "stop" {
					want = want[:3]
				}
				assert.Equal(t, want, calls)
				require.True(t, mu.TryLock(), "operation lock leaked on completion")
				mu.Unlock()
			})
		})
	}
}

func TestPackageInstallRetainsAsyncLock(t *testing.T) {
	for _, failInstall := range []bool{false, true} {
		name := "success"
		if failInstall {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			mockey.PatchConvey("async installation owns the lock until install and start finish", t, func() {
				c := NewPackageController(nil)
				pkg := &packages.PackageStatus{Name: t.Name(), TotalTime: time.Second}
				c.packageStatus[pkg.Name] = pkg
				entered := make(chan struct{})
				release := make(chan struct{})
				var installs, starts atomic.Int64
				mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
					switch arg {
					case "install":
						installs.Add(1)
						close(entered)
						<-release
						if failInstall {
							return errors.New("install failed")
						}
						return nil
					case "start":
						starts.Add(1)
						mu := packagelock.For(pkg.Name)
						if mu.TryLock() {
							mu.Unlock()
							t.Error("lock released before post-install start")
						}
						return nil
					default:
						return errors.New("not installed")
					}
				}).Build()
				c.installPackage(context.Background(), pkg)
				<-entered
				c.installPackage(context.Background(), pkg)
				c.statusPackage(context.Background(), pkg)
				c.deletePackage(context.Background(), pkg)
				c.updatePackage(context.Background(), pkg)
				assert.EqualValues(t, 1, installs.Load())
				close(release)
				mu := packagelock.For(pkg.Name)
				require.Eventually(t, func() bool {
					if !mu.TryLock() {
						return false
					}
					defer mu.Unlock()
					c.RLock()
					defer c.RUnlock()
					return !pkg.Installing && pkg.Progress == 100
				}, 5*time.Second, time.Millisecond)
				if failInstall {
					assert.Zero(t, starts.Load())
				} else {
					assert.EqualValues(t, 1, starts.Load())
				}
			})
		})
	}
}
