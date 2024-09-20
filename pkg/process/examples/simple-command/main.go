package main

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/process"
)

func main() {
	p, err := process.New(
		process.WithCommand("echo", "1"),
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

	scanner := bufio.NewScanner(p.StdoutReader())
	for scanner.Scan() { // returns false at the end of the output
		line := scanner.Text()
		fmt.Println("stdout:", line)
		select {
		case err := <-p.Wait():
			if err != nil {
				panic(err)
			}
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
			panic(err)
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
