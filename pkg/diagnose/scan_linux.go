//go:build linux
// +build linux

package diagnose

import (
	"context"
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/euank/go-kmsg-parser/v3/kmsgparser"
	"golang.org/x/sync/errgroup"

	nvidia_sxid "github.com/leptonai/gpud/components/accelerator/nvidia/error/sxid"
	nvidia_xid "github.com/leptonai/gpud/components/accelerator/nvidia/error/xid"
	"github.com/leptonai/gpud/pkg/log"
)

func scanKmsg(ctx context.Context) {
	fmt.Printf("%s scanning kmsg\n", inProgress)
	kl, err := kmsgparser.NewParser(kmsgparser.WithNoFollow())
	if err != nil {
		log.Logger.Warnw("error creating kmsg parser", "error", err)
		return
	}
	defer kl.Close()

	ch := make(chan kmsgparser.Message, 1024)

	gr, _ := errgroup.WithContext(ctx)
	gr.Go(func() error {
		return kl.Parse(ch)
	})

	cnt := 0
	now := time.Now().UTC()
	for msg := range ch {
		cnt++
		ts := humanize.RelTime(msg.Timestamp, now, "ago", "from now")

		if found := nvidia_xid.Match(msg.Message); found != nil {
			fmt.Printf("[XID found] (%s) %q\n", ts, msg.Message)
		}
		if found := nvidia_sxid.Match(msg.Message); found != nil {
			fmt.Printf("[SXID found] (%s) %q\n", ts, msg.Message)
		}
	}

	if err := gr.Wait(); err != nil {
		log.Logger.Warnw("failed to scan kmsg", "error", err)
	} else {
		fmt.Printf("%s scanned kmsg file for %d line(s)\n", checkMark, cnt)
	}
}
