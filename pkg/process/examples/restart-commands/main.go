package main

import (
	"context"
	"fmt"
	"os"
	"strings"
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

	p, err := process.New(
		process.WithCommand("echo hello"),
		process.WithCommand("echo 111 && exit 1"),
		process.WithRunAsBashScript(),
		process.WithRestartConfig(process.RestartConfig{
			OnError:  true,
			Limit:    3,
			Interval: 100 * time.Millisecond,
		}),
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

	for i := 0; i < 3; i++ {
		select {
		case err := <-p.Wait():
			if err == nil {
				panic("expected error")
			}
			if strings.Contains(err.Error(), "exit status 1") {
				fmt.Println(err)
				continue
			}
			panic(err)

		case <-time.After(2 * time.Second):
			panic("timeout")
		}
	}

	select {
	case err := <-p.Wait():
		if err != nil {
			fmt.Println("wait error:", err)
		}
	case <-time.After(2 * time.Second):
		panic("timeout")
	}

	if err := p.Abort(ctx); err != nil {
		panic(err)
	}
	if err := p.Abort(ctx); err != nil {
		panic(err)
	}

	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		panic(err)
	}
	fmt.Printf("content: %q\n", string(content))
}
