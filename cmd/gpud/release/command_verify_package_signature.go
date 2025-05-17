// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This file is based on https://github.com/tailscale/tailscale/blob/012933635b43ac41c8ff4340213bdae9abd6d059/cmd/dist/dist.go

package release

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/release/distsign"
)

func CommandVerifyPackageSignature(cliContext *cli.Context) error {
	signPubPath := cliContext.String("sign-pub-path")
	signPubBundle, err := os.ReadFile(signPubPath)
	if err != nil {
		return err
	}
	signPubs, err := distsign.ParseSigningKeyBundle(signPubBundle)
	if err != nil {
		return fmt.Errorf("parsing %q: %w", signPubPath, err)
	}
	packagePath := cliContext.String("package-path")
	pkg, err := os.Open(packagePath)
	if err != nil {
		return err
	}
	defer pkg.Close()
	pkgHash := distsign.NewPackageHash()
	if _, err := io.Copy(pkgHash, pkg); err != nil {
		return fmt.Errorf("reading %q: %w", packagePath, err)
	}
	hash := binary.LittleEndian.AppendUint64(pkgHash.Sum(nil), uint64(pkgHash.Len()))
	sigPath := cliContext.String("sig-path")
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return err
	}
	if !distsign.VerifyAny(signPubs, hash, sig) {
		return errors.New("signature not valid")
	}
	fmt.Println("signature ok")
	return nil
}
