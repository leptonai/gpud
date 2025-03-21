package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/leptonai/gpud/pkg/uptime"
)

func main() {
	var wg sync.WaitGroup
	wg.Add(runtime.NumCPU())
	for range runtime.NumCPU() {
		go func() {
			defer wg.Done()
			for {
				val, err := uptime.GetCurrentProcessStartTimeInUnixTime()
				if err != nil {
					panic(err)
				}
				fmt.Println(val)
				time.Sleep(1 * time.Second)
			}
		}()
	}
	wg.Wait()
}
