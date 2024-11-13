package command

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/leptonai/gpud/components/query/log/tail"

	"github.com/urfave/cli"
)

const logFile = "/var/log/gpud.log"

func cmdLogs(cliContext *cli.Context) error {
	if _, err := os.Stat(logFile); err != nil {
		return fmt.Errorf("log file %s does not exist", logFile)
	}

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	fmt.Printf("%s tailing %d lines\n\n", inProgress, tailLines)

	lines := make([]string, 0, tailLines)
	_, err := tail.Scan(
		rootCtx,
		tail.WithDedup(true),
		tail.WithFile(logFile),
		tail.WithLinesToTail(tailLines),
		tail.WithPerLineFunc(func(line []byte) {
			lines = append(lines, string(line))
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to tail log file: %w", err)
	}

	// print in reverse order (last line is the latest)
	for i := len(lines) - 1; i >= 0; i-- {
		fmt.Println(lines[i])
	}

	return nil
}
