// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This file is based on https://github.com/tailscale/tailscale/blob/012933635b43ac41c8ff4340213bdae9abd6d059/cmd/dist/dist.go

package release

import (
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/release/distsign"
)

func CommandVerifyKeySignature(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.SetLogger(log.CreateLogger(zapLvl, ""))

	log.Logger.Debugw("starting verify-key-signature command")

	rootPubPath := cliContext.String("root-pub-path")
	rootPubBundle, err := os.ReadFile(rootPubPath)
	if err != nil {
		return err
	}
	rootPubs, err := distsign.ParseRootKeyBundle(rootPubBundle)
	if err != nil {
		return fmt.Errorf("parsing %q: %w", rootPubPath, err)
	}
	signPubPath := cliContext.String("sign-pub-path")
	signPubBundle, err := os.ReadFile(signPubPath)
	if err != nil {
		return err
	}
	sigPath := cliContext.String("sig-path")
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return err
	}
	if !distsign.VerifyAny(rootPubs, signPubBundle, sig) {
		return errors.New("signature not valid")
	}
	fmt.Println("signature ok")
	return nil
}
