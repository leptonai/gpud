// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

package distsign

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sync"

	"github.com/leptonai/gpud/third_party/tailscale/distsign/rootkeys"
)

var roots = sync.OnceValue(func() []ed25519.PublicKey {
	roots, err := parseRoots()
	if err != nil {
		panic(err)
	}
	return roots
})

func parseRoots() ([]ed25519.PublicKey, error) {
	files, err := rootkeys.RootsFS.ReadDir("keys")
	if err != nil {
		return nil, err
	}
	var keys []ed25519.PublicKey
	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}
		if filepath.Ext(f.Name()) != ".pem" {
			continue
		}
		raw, err := rootkeys.RootsFS.ReadFile(path.Join("keys", f.Name()))
		if err != nil {
			return nil, err
		}
		key, err := parseSinglePublicKey(raw, pemTypeRootPublic)
		if err != nil {
			return nil, fmt.Errorf("parsing root key %q: %w", f.Name(), err)
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, errors.New("no embedded root keys, please check rootkeys/")
	}
	return keys, nil
}
