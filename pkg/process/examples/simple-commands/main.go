package main

import (
	"context"
	"fmt"
	"time"

	"github.com/leptonai/gpud/pkg/process"
)

func main() {
	p, err := process.New(
		process.WithCommand("echo", "1"),
		process.WithCommand("echo 2"),
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

	if err := process.ReadAllStdout(
		ctx,
		p,
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
