// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This file is based on https://github.com/tailscale/tailscale/blob/012933635b43ac41c8ff4340213bdae9abd6d059/cmd/dist/dist.go

package release

import (
	"os"

	"github.com/urfave/cli"
	"golang.org/x/crypto/blake2s"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/release/distsign"
)

func CommandSignPackage(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting sign-package command")

	signPrivPath := cliContext.String("sign-priv-path")
	signPrivRaw, err := os.ReadFile(signPrivPath)
	if err != nil {
		return err
	}
	signPrivKey, err := distsign.ParseSigningKey(signPrivRaw)
	if err != nil {
		return err
	}

	packagePath := cliContext.String("package-path")
	pkgData, err := os.ReadFile(packagePath)
	if err != nil {
		return err
	}

	hash := blake2s.Sum256(pkgData)
	sig, err := signPrivKey.SignPackageHash(hash[:], int64(len(pkgData)))
	if err != nil {
		return err
	}

	sigPath := cliContext.String("sig-path")
	if err := os.WriteFile(sigPath, sig, 0400); err != nil {
		return err
	}
	return nil
}
