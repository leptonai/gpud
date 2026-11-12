package process

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
)

func TestOutputSurvivesProcessExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := New(WithCommand("printf '%s\\n' '{\"filesystems\":[]}' ; printf 'diagnostic\\n' >&2"), WithRunAsBashScript(), WithRunBashInline())
	require.NoError(t, err)
	require.NoError(t, p.Start(ctx))
	defer func() { _ = p.Close(ctx) }()

	// Force the ordering that used to let Wait discard unread pipe data:
	// the child exited successfully, but Read returned no lines and no error.
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

func TestOutputPipeSetupFailureClosesDescriptors(t *testing.T) {
	stdout, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdout.Close(); _ = writer.Close() })
	mockey.PatchConvey("stderr pipe failure releases the stdout pipe", t, func() {
		calls := 0
		mockey.Mock(os.Pipe).To(func() (*os.File, *os.File, error) {
			calls++
			if calls == 1 {
				return stdout, writer, nil
			}
			return nil, nil, syscall.EMFILE
		}).Build()
		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)
		err = p.Start(context.Background())
		require.ErrorIs(t, err, syscall.EMFILE)
		require.ErrorContains(t, err, "stderr pipe")
		require.False(t, p.Started())
		_, err = stdout.Stat()
		require.ErrorIs(t, err, os.ErrClosed)
		_, err = writer.Stat()
		require.ErrorIs(t, err, os.ErrClosed)
	})
}

func TestOutputRestartClosesPreviousPipes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gate := filepath.Join(t.TempDir(), "exit")
	p, err := New(
		WithBashScriptContentsToRun(`while [ ! -e "$EXIT_GATE" ]; do sleep 0.01; done
printf 'attempt\n'
exit 1`),
		WithRunBashInline(), WithEnvs("EXIT_GATE="+gate),
		WithRestartConfig(RestartConfig{OnError: true, Limit: 1, Interval: time.Millisecond}),
	)
	require.NoError(t, err)
	require.NoError(t, p.Start(ctx))
	defer func() { _ = p.Close(ctx) }()
	// Hold the first child open until we retain its readers; then allow the
	// real retry loop to replace them and finish before inspecting cleanup.
	stdout, stderr := p.StdoutReader(), p.StderrReader()
	require.NoError(t, os.WriteFile(gate, nil, 0600))
	attempts := 0
wait:
	for {
		select {
		case err, ok := <-p.Wait():
			if !ok {
				break wait
			}
			require.Error(t, err)
			attempts++
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	require.Equal(t, 2, attempts)
	for _, reader := range []io.Reader{stdout, stderr} {
		_, err := io.ReadAll(reader)
		require.ErrorIs(t, err, os.ErrClosed)
	}
	// The final attempt's output must remain readable even on a nonzero exit.
	output, err := io.ReadAll(p.StdoutReader())
	require.NoError(t, err)
	require.Equal(t, "attempt\n", string(output))
}

func TestReadCanceledAtEndOfOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &mockProcess{started: true, stdoutReader: strings.NewReader("")}
	// An empty stream never enters the scan loop's cancellation check.
	require.ErrorIs(t, Read(ctx, p, WithReadStdout()), context.Canceled)
}

func TestOutputStartFailureClosesDescriptors(t *testing.T) {
	stdout, stdoutWriter, err := os.Pipe()
	require.NoError(t, err)
	stderr, stderrWriter, err := os.Pipe()
	require.NoError(t, err)
	files := []*os.File{stdout, stdoutWriter, stderr, stderrWriter}
	t.Cleanup(func() {
		for _, f := range files {
			_ = f.Close()
		}
	})
	command := filepath.Join(t.TempDir(), "command")
	require.NoError(t, os.WriteFile(command, []byte("#!/bin/sh\nexit 0\n"), 0755))
	p, err := New(WithCommand(command))
	require.NoError(t, err)
	// Executable discovery succeeds, but the executable disappears before
	// Start. Both newly allocated pipes must be released on this path.
	require.NoError(t, os.Remove(command))
	mockey.PatchConvey("failed exec closes both pipes", t, func() {
		calls := 0
		mockey.Mock(os.Pipe).To(func() (*os.File, *os.File, error) {
			calls++
			if calls == 1 {
				return stdout, stdoutWriter, nil
			}
			return stderr, stderrWriter, nil
		}).Build()
		require.ErrorIs(t, p.Start(context.Background()), os.ErrNotExist)
		require.False(t, p.Started())
		require.Nil(t, p.StdoutReader())
		require.Nil(t, p.StderrReader())
		for _, f := range files {
			_, err := f.Stat()
			require.ErrorIs(t, err, os.ErrClosed)
		}
	})
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
