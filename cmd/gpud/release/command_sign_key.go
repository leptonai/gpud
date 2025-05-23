// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This file is based on https://github.com/tailscale/tailscale/blob/012933635b43ac41c8ff4340213bdae9abd6d059/cmd/dist/dist.go

package release

import (
	"fmt"
	"os"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/release/distsign"
)

func CommandSignKey(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting sign-key command")

	rootPrivPath := cliContext.String("root-priv-path")
	rkRaw, err := os.ReadFile(rootPrivPath)
	if err != nil {
		return err
	}
	rk, err := distsign.ParseRootKey(rkRaw)
	if err != nil {
		return err
	}

	signPubPath := cliContext.String("sign-pub-path")
	bundle, err := os.ReadFile(signPubPath)
	if err != nil {
		return err
	}
	sig, err := rk.SignSigningKeys(bundle)
	if err != nil {
		return err
	}

	sigPath := cliContext.String("sig-path")
	if err := os.WriteFile(sigPath, sig, 0400); err != nil {
		return fmt.Errorf("failed writing signature file: %w", err)
	}
	fmt.Println("wrote signature to", sigPath)
	return nil
}
