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

import "github.com/prometheus/procfs"

// Returns the current process start time in unix time.
func GetCurrentProcessStartTimeInUnixTime() (uint64, error) {
	proc, err := procfs.Self()
	if err != nil {
		return 0, err
	}

	stat, err := proc.Stat()
	if err != nil {
		return 0, err
	}

	startTime, err := stat.StartTime()
	if err != nil {
		return 0, err
	}

	return uint64(startTime), nil
}
