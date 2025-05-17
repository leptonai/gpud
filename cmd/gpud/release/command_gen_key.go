// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This file is based on https://github.com/tailscale/tailscale/blob/012933635b43ac41c8ff4340213bdae9abd6d059/cmd/dist/dist.go

package release

import (
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/release/distsign"
)

func CommandGenKey(cliContext *cli.Context) error {
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
