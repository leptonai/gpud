package disk

import (
	"context"
	"io"
	"strings"

	"github.com/leptonai/gpud/pkg/process"
)

func quoteShellArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// runDiskCommand keeps diagnostics out of structured stdout, while draining
// stderr concurrently so a verbose wrapper cannot fill the pipe and stall.
func runDiskCommand(ctx context.Context, command string) (string, error) {
	p, err := process.New(process.WithCommand(command), process.WithRunAsBashScript(), process.WithRunBashInline())
	if err != nil {
		return "", err
	}
	if err := p.Start(ctx); err != nil {
		return "", err
	}
	stderr := p.StderrReader()
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.Copy(io.Discard, stderr)
	}()
	defer func() {
		_ = p.Close(ctx)
		<-drained
	}()
	var output strings.Builder
	err = process.Read(ctx, p, process.WithReadStdout(), process.WithWaitForCmd(), process.WithProcessLine(func(line string) {
		output.WriteString(line)
		output.WriteByte('\n')
	}))
	return output.String(), err
}
