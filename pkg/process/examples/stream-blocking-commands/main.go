package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/leptonai/gpud/pkg/process"
)

func main() {
	p, err := process.New(
		process.WithCommand("echo hello && sleep 1000"),
		process.WithRunAsBashScript(),
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	if err := process.Read(
		ctx,
		p,
		process.WithReadStdout(),
		process.WithReadStderr(),
		process.WithProcessLine(func(line string) {
			fmt.Println("stdout:", line)
		}),
		process.WithWaitForCmd(),
	); err != nil {
		panic(err)
	}

	if err := p.Abort(ctx); err != nil {
		panic(err)
	}
	if err := p.Abort(ctx); err != nil {
		panic(err)
	}
}
