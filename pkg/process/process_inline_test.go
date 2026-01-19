package process

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Verifies that inline bash mode runs without creating a temp file.
func TestProcessWithRunBashInline(t *testing.T) {
	p, err := New(
		WithCommand("echo", "hello"),
		WithCommand("echo inline"),
		WithRunAsBashScript(),
		WithRunBashInline(),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))

	select {
	case err := <-p.Wait():
		require.NoError(t, err, "unexpected error")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for process exit")
	}

	// Ensure no on-disk bash file was created
	if proc, ok := p.(*process); ok {
		if proc.runBashFile != nil {
			_, err := os.Stat(proc.runBashFile.Name())
			require.Error(t, err, "expected no temp bash file to exist, but found: %s", proc.runBashFile.Name())
		}
	}
}

func TestProcessWithRunBashInline_QuotesAndMeta(t *testing.T) {
	// Ensure tricky quoting works fine via stdin (-s), not -c.
	tmp, err := os.CreateTemp("", "process-inline-*.log")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(tmp.Name())
	}()
	defer func() {
		_ = tmp.Close()
	}()

	p, err := New(
		WithBashScriptContentsToRun(`echo 'hello "world"' && echo a|tr 'a' 'A'`),
		WithRunAsBashScript(),
		WithRunBashInline(),
		WithOutputFile(tmp),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))

	select {
	case err := <-p.Wait():
		require.NoError(t, err, "unexpected error")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for process exit")
	}

	require.NoError(t, p.Close(ctx))

	b, err := os.ReadFile(tmp.Name())
	require.NoError(t, err)

	out := string(b)
	require.Contains(t, out, "hello \"world\"", "expected quoted output")
	require.Contains(t, out, "A", "expected pipeline output with capital A")
}

func TestProcessWithRunBashInline_MultiCommand(t *testing.T) {
	// Use command list; runtime assembles bash program + headers and feeds stdin.
	tmp, err := os.CreateTemp("", "process-inline-*.log")
	require.NoError(t, err)
	defer func() {
		_ = os.Remove(tmp.Name())
	}()
	defer func() {
		_ = tmp.Close()
	}()

	p, err := New(
		WithCommand("echo", "hello world"),
		WithCommand("echo hello && echo 111 | grep 1"),
		WithRunAsBashScript(),
		WithRunBashInline(),
		WithOutputFile(tmp),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))

	select {
	case err := <-p.Wait():
		require.NoError(t, err, "unexpected error")
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for process exit")
	}

	require.NoError(t, p.Close(ctx))

	b, err := os.ReadFile(tmp.Name())
	require.NoError(t, err)

	out := string(b)
	require.Contains(t, out, "hello world", "missing 'hello world' in output")
	require.Contains(t, out, "111", "missing '111' in output")
}
