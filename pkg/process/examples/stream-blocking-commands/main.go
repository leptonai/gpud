package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
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

	scanner := bufio.NewScanner(p.StdoutReader())
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()
		fmt.Println("stdout:", line)
		select {
		case err := <-p.Wait():
			if err != nil {
				panic(err)
			}
		case sig := <-sigChan:
			fmt.Printf("Received signal %s, exiting...\n", sig)
		default:
		}
	}
	if serr := scanner.Err(); serr != nil {
		// process already dead, thus ignore
		// e.g., "read |0: file already closed"
		if !strings.Contains(serr.Error(), "file already closed") {
			panic(serr)
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
}
