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

// modified from https://github.com/prometheus/procfs/blob/master/sysfs/class_infiniband_test.go

package class

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfiniBandClass(t *testing.T) {
	fs, err := loadSysFS("testdata/sys-class-infiniband-h100.0")
	assert.NoError(t, err)

	ibc, err := fs.InfiniBandClass()
	assert.NoError(t, err)

	assert.Equal(t, 9, len(ibc))
	assert.Equal(t, "mlx5_0", ibc["mlx5_0"].Name)
	assert.Equal(t, "mlx5_0", ibc["mlx5_0"].Name)
}
