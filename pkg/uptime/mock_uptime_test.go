// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux
// +build linux

package uptime

import (
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetCurrentProcessStartTimeInUnixTime_ProcfsSelfError tests error handling when procfs.Self() fails.
func TestGetCurrentProcessStartTimeInUnixTime_ProcfsSelfError(t *testing.T) {
	mockey.PatchConvey("procfs.Self error", t, func() {
		mockey.Mock(procfs.Self).To(func() (procfs.Proc, error) {
			return procfs.Proc{}, errors.New("failed to get self proc")
		}).Build()

		startTime, err := GetCurrentProcessStartTimeInUnixTime()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get self proc")
		assert.Equal(t, uint64(0), startTime)
	})
}

// TestGetCurrentProcessStartTimeInUnixTime_ProcStatError tests error handling when proc.Stat() fails.
func TestGetCurrentProcessStartTimeInUnixTime_ProcStatError(t *testing.T) {
	mockey.PatchConvey("proc.Stat error", t, func() {
		mockProc := procfs.Proc{}

		mockey.Mock(procfs.Self).To(func() (procfs.Proc, error) {
			return mockProc, nil
		}).Build()

		mockey.Mock((procfs.Proc).Stat).To(func() (procfs.ProcStat, error) {
			return procfs.ProcStat{}, errors.New("failed to get proc stat")
		}).Build()

		startTime, err := GetCurrentProcessStartTimeInUnixTime()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get proc stat")
		assert.Equal(t, uint64(0), startTime)
	})
}

// TestGetCurrentProcessStartTimeInUnixTime_StartTimeError tests error handling when stat.StartTime() fails.
func TestGetCurrentProcessStartTimeInUnixTime_StartTimeError(t *testing.T) {
	mockey.PatchConvey("stat.StartTime error", t, func() {
		mockProc := procfs.Proc{}
		mockStat := procfs.ProcStat{}

		mockey.Mock(procfs.Self).To(func() (procfs.Proc, error) {
			return mockProc, nil
		}).Build()

		mockey.Mock((procfs.Proc).Stat).To(func() (procfs.ProcStat, error) {
			return mockStat, nil
		}).Build()

		mockey.Mock((procfs.ProcStat).StartTime).To(func() (float64, error) {
			return 0, errors.New("failed to get start time")
		}).Build()

		startTime, err := GetCurrentProcessStartTimeInUnixTime()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get start time")
		assert.Equal(t, uint64(0), startTime)
	})
}

// TestGetCurrentProcessStartTimeInUnixTime_Success tests successful execution.
func TestGetCurrentProcessStartTimeInUnixTime_Success(t *testing.T) {
	mockey.PatchConvey("successful execution", t, func() {
		mockProc := procfs.Proc{}
		mockStat := procfs.ProcStat{}

		mockey.Mock(procfs.Self).To(func() (procfs.Proc, error) {
			return mockProc, nil
		}).Build()

		mockey.Mock((procfs.Proc).Stat).To(func() (procfs.ProcStat, error) {
			return mockStat, nil
		}).Build()

		// Return a typical Unix timestamp (e.g., Jan 1, 2024 00:00:00 UTC = 1704067200)
		expectedStartTime := float64(1704067200)
		mockey.Mock((procfs.ProcStat).StartTime).To(func() (float64, error) {
			return expectedStartTime, nil
		}).Build()

		startTime, err := GetCurrentProcessStartTimeInUnixTime()
		require.NoError(t, err)
		assert.Equal(t, uint64(expectedStartTime), startTime)
	})
}

// TestGetCurrentProcessStartTimeInUnixTime_SuccessWithLargeTimestamp tests successful execution with a large timestamp.
func TestGetCurrentProcessStartTimeInUnixTime_SuccessWithLargeTimestamp(t *testing.T) {
	mockey.PatchConvey("successful execution with large timestamp", t, func() {
		mockProc := procfs.Proc{}
		mockStat := procfs.ProcStat{}

		mockey.Mock(procfs.Self).To(func() (procfs.Proc, error) {
			return mockProc, nil
		}).Build()

		mockey.Mock((procfs.Proc).Stat).To(func() (procfs.ProcStat, error) {
			return mockStat, nil
		}).Build()

		// Return a large Unix timestamp to verify uint64 conversion works correctly
		expectedStartTime := float64(1893456000) // Jan 1, 2030
		mockey.Mock((procfs.ProcStat).StartTime).To(func() (float64, error) {
			return expectedStartTime, nil
		}).Build()

		startTime, err := GetCurrentProcessStartTimeInUnixTime()
		require.NoError(t, err)
		assert.Equal(t, uint64(expectedStartTime), startTime)
	})
}

// TestGetCurrentProcessStartTimeInUnixTime_SuccessWithZeroTimestamp tests handling of zero timestamp.
func TestGetCurrentProcessStartTimeInUnixTime_SuccessWithZeroTimestamp(t *testing.T) {
	mockey.PatchConvey("successful execution with zero timestamp", t, func() {
		mockProc := procfs.Proc{}
		mockStat := procfs.ProcStat{}

		mockey.Mock(procfs.Self).To(func() (procfs.Proc, error) {
			return mockProc, nil
		}).Build()

		mockey.Mock((procfs.Proc).Stat).To(func() (procfs.ProcStat, error) {
			return mockStat, nil
		}).Build()

		mockey.Mock((procfs.ProcStat).StartTime).To(func() (float64, error) {
			return 0, nil
		}).Build()

		startTime, err := GetCurrentProcessStartTimeInUnixTime()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), startTime)
	})
}
