// SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package packagelock serializes in-process operations on managed packages.
package packagelock

import "sync"

var locks sync.Map

// For returns the process-wide operation mutex for a package name.
// Entries are retained so all callers always share the same mutex.
func For(name string) *sync.Mutex {
	lock, _ := locks.LoadOrStore(name, &sync.Mutex{})
	return lock.(*sync.Mutex)
}
