package disk

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/leptonai/gpud/pkg/process"
)

func quoteShellArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// runDiskCommand keeps diagnostics out of structured stdout, while draining
// stderr concurrently so a verbose wrapper cannot fill the pipe and stall.
func runDiskCommand(ctx context.Context, command string) (string, error) {
	output, _, err := runDiskCommandOutput(ctx, command)
	return output, err
}

// Diagnostics are bounded, but the entire stderr stream is drained.
func runDiskCommandOutput(ctx context.Context, command string) (output, diagnostics string, err error) {
	// Keep structured output and the empty-filter diagnostic locale-stable.
	envs := make([]string, 0, len(os.Environ())+1)
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "LC_ALL=") {
			envs = append(envs, env)
		}
	}
	envs = append(envs, "LC_ALL=C")
	p, err := process.New(process.WithCommand(command), process.WithEnvs(envs...), process.WithRunAsBashScript(), process.WithRunBashInline())
	if err != nil {
		return "", "", err
	}
	if err := p.Start(ctx); err != nil {
		return "", "", err
	}
	stderr := p.StderrReader()
	var stderrOutput strings.Builder
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		_, _ = io.CopyN(&stderrOutput, stderr, 4096)
		_, _ = io.Copy(io.Discard, stderr)
	}()
	defer func() {
		// On failure, read diagnostics before Close discards unread pipe data.
		// The caller's context still bounds descendants retaining stderr.
		if err != nil {
			select {
			case <-drained:
			case <-ctx.Done():
			}
		}
		_ = p.Close(ctx)
		<-drained
		diagnostics = stderrOutput.String()
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	var stdout strings.Builder
	err = process.Read(ctx, p, process.WithReadStdout(), process.WithWaitForCmd(), process.WithProcessLine(func(line string) {
		stdout.WriteString(line)
		stdout.WriteByte('\n')
	}))
	return stdout.String(), "", err
}
