package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/leptonai/gpud/pkg/process"
)

func main() {
	// create a temporary file
	tmpFile, err := os.CreateTemp("", "process-test-*.txt")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// NOTE: stderr/stdout piping sometimes doesn't work on mac
	// if run as a bash script
	p, err := process.New(
		process.WithBashScriptContentsToRun(`#!/bin/bash

# do not mask errors in a pipeline
set -o pipefail

echo hello
`),
		process.WithOutputFile(tmpFile),
	)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Start(ctx); err != nil {
		panic(err)
	}
	fmt.Printf("pid: %d\n", p.PID())

	select {
	case err := <-p.Wait():
		if err != nil {
			panic(err)
		}
	case <-time.After(2 * time.Second):
		panic("timeout")
	}

	if err := p.Close(ctx); err != nil {
		panic(err)
	}
	if err := p.Close(ctx); err != nil {
		panic(err)
	}

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		panic(err)
	}
	fmt.Printf("content: %q\n", string(content))
}
