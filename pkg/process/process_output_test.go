package process

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOutputSurvivesProcessExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := New(WithCommand("printf '%s\\n' '{\"filesystems\":[]}' ; printf 'diagnostic\\n' >&2"), WithRunAsBashScript(), WithRunBashInline())
	require.NoError(t, err)
	require.NoError(t, p.Start(ctx))
	defer func() { _ = p.Close(ctx) }()

	// Force the ordering that used to let Wait discard unread pipe data.
	select {
	case err := <-p.Wait():
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	var lines []string
	require.NoError(t, Read(ctx, p, WithReadStdout(), WithReadStderr(), WithWaitForCmd(), WithProcessLine(func(s string) {
		lines = append(lines, s)
	})))
	require.Equal(t, []string{`{"filesystems":[]}`, "diagnostic"}, lines)
}

func TestOutputCancellationUnblocksReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, err := New(WithCommand("sleep", "30"))
	require.NoError(t, err)
	require.NoError(t, p.Start(ctx))
	defer func() { _ = p.Close(context.Background()) }()
	done := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, p.StdoutReader()); done <- err }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reader remained blocked after cancellation")
	}
}

func TestOutputLargerThanPipe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := New(WithBashScriptContentsToRun("i=0; while [ $i -lt 10000 ]; do printf 'complete output line\\n'; i=$((i+1)); done"), WithRunBashInline())
	require.NoError(t, err)
	require.NoError(t, p.Start(ctx))
	defer func() { _ = p.Close(ctx) }()
	var output strings.Builder
	require.NoError(t, Read(ctx, p, WithReadStdout(), WithWaitForCmd(), WithProcessLine(func(s string) {
		output.WriteString(s + "\n")
	})))
	require.Equal(t, strings.Repeat("complete output line\n", 10000), output.String())
}
