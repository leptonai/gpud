// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package packagelock

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestForSharesMutexByPackage(t *testing.T) {
	mu := For(t.Name())
	mu.Lock()
	defer mu.Unlock()
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := For(t.Name())
			if other.TryLock() {
				other.Unlock()
				t.Error("same package acquired a second operation lock")
			}
		}()
	}
	wg.Wait()
	other := For(t.Name() + "-other")
	require.True(t, other.TryLock())
	other.Unlock()
}
