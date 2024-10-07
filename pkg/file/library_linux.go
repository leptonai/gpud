//go:build linux
// +build linux

/**
# Copyright 2024 NVIDIA CORPORATION
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package file

import (
	"os"
	"path/filepath"
)

// ref. "github.com/NVIDIA/go-nvml/pkg/nvlib/info/root.go"
// ref. https://github.com/NVIDIA/nvidia-container-toolkit/blob/main/internal/lookup/library.go#L33-L62
var defaultLibSearchDirs = map[string]any{
	"/usr/lib64":                 struct{}{},
	"/usr/lib/x86_64-linux-gnu":  struct{}{},
	"/usr/lib/aarch64-linux-gnu": struct{}{},
	"/lib64":                     struct{}{},
	"/lib/x86_64-linux-gnu":      struct{}{},
	"/lib/aarch64-linux-gnu":     struct{}{},
}

// Returns the resolved path of the library if found.
// Returns an empty string and no error if not found.
func findLibrary(searchDirs map[string]any, name string) (string, error) {
	if name == "" {
		return "", ErrLibraryEmpty
	}

	for dir := range searchDirs {
		libPath := filepath.Join(dir, name)
		if _, err := os.Stat(libPath); err != nil {
			if !os.IsNotExist(err) {
				return "", err
			}
			// does not exist, continue
			continue
		}
		return filepath.EvalSymlinks(libPath)
	}

	return "", nil
}
