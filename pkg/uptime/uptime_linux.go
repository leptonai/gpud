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
	"sync"

	"github.com/prometheus/procfs"
)

var gpudStartTime uint64
var getStartTimeErr error
var once sync.Once

// Returns the current process start time in unix time.
func GetCurrentProcessStartTimeInUnixTime() (uint64, error) {
	once.Do(func() {
		proc, err := procfs.Self()
		if err != nil {
			getStartTimeErr = err
			return
		}

		stat, err := proc.Stat()
		if err != nil {
			getStartTimeErr = err
			return
		}

		startTime, err := stat.StartTime()
		if err != nil {
			getStartTimeErr = err
			return
		}

		gpudStartTime = uint64(startTime)
	})
	return gpudStartTime, getStartTimeErr
}
