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
	"github.com/leptonai/gpud/pkg/packagelock"
)

func TestCanceledPackageOperationsDoNotExecuteCommands(t *testing.T) {
	mockey.PatchConvey("canceled work is a no-op and does not retain ownership", t, func() {
		c := NewPackageController(nil)
		pkg := &packages.PackageStatus{Name: t.Name(), IsInstalled: true}
		c.packageStatus[pkg.Name] = pkg
		var calls atomic.Int64
		mockey.Mock(runCommand).To(func(context.Context, string, string, *string) error {
			calls.Add(1)
			return nil
		}).Build()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for _, operation := range []func(context.Context, *packages.PackageStatus){
			c.statusPackage, c.installPackage, c.updatePackage, c.deletePackage,
		} {
			operation(ctx, pkg)
			mu := packagelock.For(pkg.Name)
			require.True(t, mu.TryLock(), "canceled operation retained ownership")
			mu.Unlock()
		}
		assert.Zero(t, calls.Load())
		c.statusPackage(context.Background(), pkg)
		assert.Positive(t, calls.Load(), "subsequent live work must execute")
	})
}

func TestCanceledAsyncPackageInstallCanRetry(t *testing.T) {
	mockey.PatchConvey("canceling an active installation permits a fresh retry", t, func() {
		c := NewPackageController(nil)
		pkg := &packages.PackageStatus{Name: t.Name(), TotalTime: time.Second}
		c.packageStatus[pkg.Name] = pkg
		var installs, starts atomic.Int64
		entered := make(chan struct{})
		mockey.Mock(runCommand).To(func(ctx context.Context, script, arg string, result *string) error {
			switch arg {
			case "install":
				if installs.Add(1) == 1 {
					close(entered)
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			case "start":
				starts.Add(1)
				return nil
			default:
				return errors.New("not installed")
			}
		}).Build()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.installPackage(ctx, pkg)
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("installation never started")
		}
		cancel()
		mu := packagelock.For(pkg.Name)
		waitReleased := func() {
			t.Helper()
			require.Eventually(t, func() bool {
				if !mu.TryLock() {
					return false
				}
				defer mu.Unlock()
				c.RLock()
				defer c.RUnlock()
				return !pkg.Installing
			}, 5*time.Second, time.Millisecond)
		}
		waitReleased()
		assert.Zero(t, starts.Load(), "canceled installation must not start the package")
		c.installPackage(context.Background(), pkg)
		waitReleased()
		assert.EqualValues(t, 2, installs.Load())
		assert.EqualValues(t, 1, starts.Load())
	})
}
