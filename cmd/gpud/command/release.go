// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This file is based on https://github.com/tailscale/tailscale/blob/012933635b43ac41c8ff4340213bdae9abd6d059/cmd/dist/dist.go

package command

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli"
	"golang.org/x/crypto/blake2s"

	"github.com/leptonai/gpud/pkg/release/distsign"
)

func cmdReleaseGenKey(cliContext *cli.Context) error {
	root := cliContext.Bool("root")
	signing := cliContext.Bool("signing")
	var pub, priv []byte
	var err error
	switch {
	case root && signing:
		return errors.New("only one of --root or --signing can be set")
	case !root && !signing:
		return errors.New("set either --root or --signing")
	case root:
		priv, pub, err = distsign.GenerateRootKey()
	case signing:
		priv, pub, err = distsign.GenerateSigningKey()
	}
	if err != nil {
		fmt.Printf("failed to generate key pair: %v\n", err)
		return err
	}

	privPath := cliContext.String("priv-path")
	if err := os.WriteFile(privPath, priv, 0400); err != nil {
		return fmt.Errorf("failed writing private key: %w", err)
	}
	fmt.Println("wrote private key to", privPath)

	pubPath := cliContext.String("pub-path")
	if err := os.WriteFile(pubPath, pub, 0400); err != nil {
		return fmt.Errorf("failed writing public key: %w", err)
	}
	fmt.Println("wrote public key to", pubPath)

	return nil
}

func cmdReleaseSignKey(cliContext *cli.Context) error {
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

func cmdReleaseVerifyKeySignature(cliContext *cli.Context) error {
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

func cmdReleaseSignPackage(cliContext *cli.Context) error {
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

func cmdReleaseVerifyPackageSignature(cliContext *cli.Context) error {
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
