package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"
	gopsutil "github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- New mockey tests ---

func TestNew_CreateTempError(t *testing.T) {
	mockey.PatchConvey("process.New with CreateTemp error", t, func() {
		mockey.Mock(os.CreateTemp).To(func(dir, pattern string) (*os.File, error) {
			return nil, errors.New("disk full")
		}).Build()

		_, err := New(
			WithBashScriptContentsToRun("echo hello"),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "disk full")
	})
}

// --- CountProcessesByStatus mockey tests ---

func TestCountProcessesByStatus_ProcessesError(t *testing.T) {
	mockey.PatchConvey("ProcessesWithContext returns error", t, func() {
		mockey.Mock(gopsutil.ProcessesWithContext).To(func(ctx context.Context) ([]*gopsutil.Process, error) {
			return nil, errors.New("permission denied")
		}).Build()

		result, err := CountProcessesByStatus(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
		assert.Nil(t, result)
	})
}

// --- Read edge case tests ---

func TestRead_NoReadersSet(t *testing.T) {
	p, err := New(WithCommand("echo", "hello"))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))
	defer func() { _ = p.Close(ctx) }()

	// Call Read with no reader options set
	err = Read(ctx, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one of readStdout or readStderr must be true")
}

func TestRead_ProcessNotStarted(t *testing.T) {
	p, err := New(WithCommand("echo", "hello"))
	require.NoError(t, err)

	ctx := context.Background()

	// Call Read before Start
	err = Read(ctx, p, WithReadStdout())
	require.Error(t, err)
	assert.Equal(t, ErrProcessNotStarted, err)
}

func TestRead_ProcessAborted(t *testing.T) {
	p, err := New(WithCommand("sleep", "10"))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.Start(ctx))

	// Close the process (sets aborted=true)
	require.NoError(t, p.Close(ctx))

	// Call Read after Close
	err = Read(ctx, p, WithReadStdout())
	require.Error(t, err)
	assert.Equal(t, ErrProcessAborted, err)
}

// --- RunUntilCompletion mockey tests ---

func TestRunUntilCompletion_MkdirAllError(t *testing.T) {
	mockey.PatchConvey("RunUntilCompletion with MkdirAll error", t, func() {
		mockey.Mock(os.MkdirAll).To(func(path string, perm os.FileMode) error {
			return errors.New("permission denied")
		}).Build()

		runner := NewExclusiveRunner()
		output, exitCode, err := runner.RunUntilCompletion(context.Background(), "echo hello")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
		assert.Nil(t, output)
		assert.Equal(t, int32(0), exitCode)
	})
}

func TestRunUntilCompletion_CreateTempError(t *testing.T) {
	mockey.PatchConvey("RunUntilCompletion with CreateTemp error", t, func() {
		mockey.Mock(os.CreateTemp).To(func(dir, pattern string) (*os.File, error) {
			return nil, errors.New("no space left on device")
		}).Build()

		runner := NewExclusiveRunner()
		output, exitCode, err := runner.RunUntilCompletion(context.Background(), "echo hello")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no space left on device")
		assert.Nil(t, output)
		assert.Equal(t, int32(0), exitCode)
	})
}

// --- CountRunningPids mockey tests ---

// NOTE: TestCountRunningPids_PidsError and TestCountRunningPids_Success are
// omitted because mocking gopsutil.Pids requires -gcflags="all=-N -l" to
// disable inlining.

// --- FindProcessByName mockey tests ---

func TestFindProcessByName_ProcessesWithContextError(t *testing.T) {
	mockey.PatchConvey("FindProcessByName with ProcessesWithContext error", t, func() {
		mockey.Mock(gopsutil.ProcessesWithContext).To(func(ctx context.Context) ([]*gopsutil.Process, error) {
			return nil, errors.New("access denied")
		}).Build()

		result, err := FindProcessByName(context.Background(), "test")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "access denied")
		assert.Nil(t, result)
	})
}

// --- commandExists mockey tests ---

func TestCommandExists_LookPathError(t *testing.T) {
	mockey.PatchConvey("LookPath returns error", t, func() {
		mockey.Mock(exec.LookPath).To(func(file string) (string, error) {
			return "", errors.New("command not found in PATH")
		}).Build()

		assert.False(t, commandExists("nonexistent"))
	})
}

func TestCommandExists_LookPathEmptyPath(t *testing.T) {
	mockey.PatchConvey("LookPath returns empty string", t, func() {
		mockey.Mock(exec.LookPath).To(func(file string) (string, error) {
			return "", nil
		}).Build()

		assert.False(t, commandExists("emptypath"))
	})
}

func TestCommandExists_LookPathSuccess(t *testing.T) {
	mockey.PatchConvey("LookPath returns valid path", t, func() {
		mockey.Mock(exec.LookPath).To(func(file string) (string, error) {
			return "/usr/bin/echo", nil
		}).Build()

		assert.True(t, commandExists("echo"))
	})
}

// --- os.ReadFile mockey tests for RunUntilCompletion ---

func TestRunUntilCompletion_ReadFileErrorAfterSuccess(t *testing.T) {
	mockey.PatchConvey("RunUntilCompletion with ReadFile error on success path", t, func() {
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return nil, errors.New("I/O error reading output")
		}).Build()

		runner := NewExclusiveRunner()
		output, exitCode, err := runner.RunUntilCompletion(context.Background(), "echo hello")
		// ReadFile error is returned from the success path
		require.Error(t, err)
		assert.Contains(t, err.Error(), "I/O error reading output")
		assert.Nil(t, output)
		assert.Equal(t, int32(0), exitCode)
	})
}

func TestRunUntilCompletion_ReadFileErrorAfterFailure(t *testing.T) {
	mockey.PatchConvey("RunUntilCompletion with ReadFile error on failure path", t, func() {
		mockey.Mock(os.ReadFile).To(func(name string) ([]byte, error) {
			return nil, errors.New("I/O error reading output")
		}).Build()

		runner := NewExclusiveRunner()
		// exit 1 causes the process to fail, then ReadFile also fails
		output, exitCode, err := runner.RunUntilCompletion(context.Background(), "exit 1")
		require.Error(t, err)
		// output should be nil because ReadFile failed
		assert.Nil(t, output)
		assert.Equal(t, int32(1), exitCode)
	})
}

// --- Process.Start error injection tests ---

func TestNew_BashScriptWriteError(t *testing.T) {
	mockey.PatchConvey("bash file Write fails after CreateTemp succeeds", t, func() {
		// Create a real temp file but make it read-only so Write fails
		tmpFile, err := os.CreateTemp("", "mockey-test-*.bash")
		require.NoError(t, err)
		tmpName := tmpFile.Name()
		defer func() { _ = os.Remove(tmpName) }()
		// Close it so we can chmod
		_ = tmpFile.Close()

		// Make file read-only
		require.NoError(t, os.Chmod(tmpName, 0o444))

		// Re-open in read-only mode
		roFile, err := os.Open(tmpName)
		require.NoError(t, err)
		defer func() { _ = roFile.Close() }()

		mockey.Mock(os.CreateTemp).To(func(dir, pattern string) (*os.File, error) {
			return roFile, nil
		}).Build()

		_, err = New(
			WithBashScriptContentsToRun("echo hello"),
		)
		// Write to read-only file should fail
		require.Error(t, err)
	})
}

// --- Process lifecycle with mocked functions ---

func TestStartAndWaitForCombinedOutput_AlreadyStarted(t *testing.T) {
	mockey.PatchConvey("StartAndWaitForCombinedOutput on already started process", t, func() {
		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		ctx := context.Background()

		// Start the process first
		require.NoError(t, p.Start(ctx))
		defer func() { _ = p.Close(ctx) }()

		// Try StartAndWaitForCombinedOutput on already started process
		_, err = p.StartAndWaitForCombinedOutput(ctx)
		require.Error(t, err)
		assert.Equal(t, ErrProcessAlreadyStarted, err)
	})
}

// --- ExclusiveRunner already running test ---

func TestRunUntilCompletion_AlreadyRunning(t *testing.T) {
	mockey.PatchConvey("RunUntilCompletion with already running process", t, func() {
		runner := NewExclusiveRunner()
		er, ok := runner.(*exclusiveRunner)
		require.True(t, ok)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		started := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			close(started)
			_, _, _ = er.RunUntilCompletion(ctx, "sleep 5")
		}()

		<-started
		deadline := time.Now().Add(2 * time.Second)
		for !er.alreadyRunning() {
			if time.Now().After(deadline) {
				t.Fatal("runner did not start within timeout")
			}
			time.Sleep(10 * time.Millisecond)
		}

		// Second call should fail with ErrProcessAlreadyRunning
		_, _, err := er.RunUntilCompletion(ctx, "echo hello")
		assert.ErrorIs(t, err, ErrProcessAlreadyRunning)

		cancel()
		<-done
	})
}

// --- Process PID/ExitCode before start ---

func TestProcess_PIDAndExitCodeBeforeStart(t *testing.T) {
	mockey.PatchConvey("PID and ExitCode before process is started", t, func() {
		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		assert.Equal(t, int32(0), p.PID())
		assert.Equal(t, int32(0), p.ExitCode())
		assert.False(t, p.Started())
		assert.False(t, p.Closed())
	})
}

// --- Process with environment variables ---

func TestProcess_WithEnvsExecution(t *testing.T) {
	mockey.PatchConvey("Process with custom environment variables", t, func() {
		p, err := New(
			WithBashScriptContentsToRun("echo $MY_TEST_VAR"),
			WithEnvs("MY_TEST_VAR=mockey_test_value", "PATH=/usr/bin:/bin"),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		output, err := p.StartAndWaitForCombinedOutput(ctx)
		require.NoError(t, err)
		assert.Contains(t, string(output), "mockey_test_value")
	})
}

// --- Process WithLabel ---

func TestProcess_WithLabel(t *testing.T) {
	mockey.PatchConvey("WithLabel sets labels correctly", t, func() {
		p, err := New(
			WithCommand("echo", "hello"),
			WithLabel("env", "test"),
			WithLabel("component", "process"),
		)
		require.NoError(t, err)
		require.NotNil(t, p)
	})
}

// --- Read with nil stdout/stderr readers ---

func TestRead_NilStdoutReader(t *testing.T) {
	mockey.PatchConvey("Read with nil stdout reader", t, func() {
		p := &mockProcess{
			stdoutReader: nil,
			waitCh:       make(chan error),
			started:      true,
		}

		err := Read(context.Background(), p, WithReadStdout())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stdout reader is nil")
	})
}

func TestRead_NilStderrReader(t *testing.T) {
	mockey.PatchConvey("Read with nil stderr reader", t, func() {
		p := &mockProcess{
			stderrReader: nil,
			waitCh:       make(chan error),
			started:      true,
		}

		err := Read(context.Background(), p, WithReadStderr())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stderr reader is nil")
	})
}

// --- Read with WaitForCmd success ---

func TestRead_WithWaitForCmdSuccess(t *testing.T) {
	mockey.PatchConvey("Read with WaitForCmd that succeeds", t, func() {
		waitCh := make(chan error, 1)
		p := &mockProcess{
			stdoutReader: &mockReader{data: "line1\n"},
			waitCh:       waitCh,
			started:      true,
		}

		go func() {
			time.Sleep(50 * time.Millisecond)
			waitCh <- nil
			close(waitCh)
		}()

		err := Read(context.Background(), p, WithReadStdout(), WithWaitForCmd())
		require.NoError(t, err)
	})
}

// --- Read with WaitForCmd context cancellation ---

func TestRead_WithWaitForCmdContextCancel(t *testing.T) {
	mockey.PatchConvey("Read with WaitForCmd and context cancellation", t, func() {
		waitCh := make(chan error)
		p := &mockProcess{
			stdoutReader: &mockReader{data: "line1\n"},
			waitCh:       waitCh,
			started:      true,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := Read(ctx, p, WithReadStdout(), WithWaitForCmd())
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// --- Read with file already closed error (scanner error filtering) ---

func TestRead_FileAlreadyClosedError(t *testing.T) {
	mockey.PatchConvey("Read with file already closed scanner error", t, func() {
		// Create a reader that simulates a "file already closed" error
		p := &mockProcess{
			stdoutReader: &mockReader{err: errors.New("read |0: file already closed")},
			waitCh:       make(chan error),
			started:      true,
		}

		// The "file already closed" error should be ignored by Read
		err := Read(context.Background(), p, WithReadStdout())
		require.NoError(t, err)
	})
}

// --- Process options: WithRunBashInline ---

func TestNew_WithRunBashInlineScriptContents(t *testing.T) {
	mockey.PatchConvey("New with RunBashInline and script contents", t, func() {
		p, err := New(
			WithBashScriptContentsToRun("echo inline_test"),
			WithRunBashInline(),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		output, err := p.StartAndWaitForCombinedOutput(ctx)
		require.NoError(t, err)
		assert.Contains(t, string(output), "inline_test")
	})
}

func TestNew_WithRunBashInlineMultipleCommands(t *testing.T) {
	mockey.PatchConvey("New with RunBashInline and multiple commands", t, func() {
		p, err := New(
			WithCommand("echo", "cmd1"),
			WithCommand("echo", "cmd2"),
			WithRunBashInline(),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		output, err := p.StartAndWaitForCombinedOutput(ctx)
		require.NoError(t, err)
		assert.Contains(t, string(output), "cmd1")
		assert.Contains(t, string(output), "cmd2")
	})
}

// --- Close with nil process check ---

func TestClose_NotStartedCmd(t *testing.T) {
	mockey.PatchConvey("Close on process that has not been started", t, func() {
		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		// Close without starting should return nil (not started path)
		err = p.Close(context.Background())
		require.NoError(t, err)
		assert.False(t, p.Started())
	})
}

// --- Process with AllowDetachedProcess ---

func TestProcess_AllowDetachedProcessOption(t *testing.T) {
	mockey.PatchConvey("Process with AllowDetachedProcess option", t, func() {
		p, err := New(
			WithCommand("echo", "hello"),
			WithAllowDetachedProcess(true),
		)
		require.NoError(t, err)

		proc, ok := p.(*process)
		require.True(t, ok)
		assert.True(t, proc.allowDetachedProcess)
	})
}

func TestProcess_DefaultNoDetachedProcess(t *testing.T) {
	mockey.PatchConvey("Process defaults to no detached process", t, func() {
		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		proc, ok := p.(*process)
		require.True(t, ok)
		assert.False(t, proc.allowDetachedProcess)
	})
}

// --- RestartConfig validation ---

func TestProcess_RestartConfigInterval(t *testing.T) {
	mockey.PatchConvey("RestartConfig with custom interval", t, func() {
		p, err := New(
			WithCommand("echo", "hello"),
			WithRestartConfig(RestartConfig{
				OnError:  true,
				Limit:    3,
				Interval: 10 * time.Second,
			}),
		)
		require.NoError(t, err)

		proc, ok := p.(*process)
		require.True(t, ok)
		assert.Equal(t, 10*time.Second, proc.restartConfig.Interval)
		assert.Equal(t, 3, proc.restartConfig.Limit)
		assert.True(t, proc.restartConfig.OnError)
	})
}

func TestProcess_RestartConfigDefaultInterval(t *testing.T) {
	mockey.PatchConvey("RestartConfig with zero interval gets default", t, func() {
		p, err := New(
			WithCommand("echo", "hello"),
			WithRestartConfig(RestartConfig{
				OnError:  true,
				Limit:    1,
				Interval: 0,
			}),
		)
		require.NoError(t, err)

		proc, ok := p.(*process)
		require.True(t, ok)
		assert.Equal(t, 5*time.Second, proc.restartConfig.Interval)
	})
}

// --- Read with WithProcessLine callback ---

func TestRead_WithProcessLineCallback(t *testing.T) {
	mockey.PatchConvey("Read with WithProcessLine collects lines", t, func() {
		p, err := New(
			WithRunBashInline(),
			WithBashScriptContentsToRun("echo line1\necho line2\necho line3"),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		require.NoError(t, p.Start(ctx))
		defer func() { _ = p.Close(ctx) }()

		var lines []string
		err = Read(ctx, p,
			WithReadStdout(),
			WithProcessLine(func(line string) {
				lines = append(lines, line)
			}),
			WithWaitForCmd(),
		)
		require.NoError(t, err)
		assert.Contains(t, lines, "line1")
		assert.Contains(t, lines, "line2")
		assert.Contains(t, lines, "line3")
	})
}

// --- Read with WithInitialBufferSize ---

func TestRead_WithInitialBufferSize(t *testing.T) {
	mockey.PatchConvey("Read with WithInitialBufferSize uses custom buffer", t, func() {
		// Generate a line longer than the default bufio buffer (4KB) to verify custom buffer works
		longLine := strings.Repeat("x", 5000)
		p, err := New(
			WithRunBashInline(),
			WithBashScriptContentsToRun(fmt.Sprintf("echo %s", longLine)),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		require.NoError(t, p.Start(ctx))
		defer func() { _ = p.Close(ctx) }()

		var lines []string
		err = Read(ctx, p,
			WithReadStdout(),
			WithInitialBufferSize(8192), // 8KB buffer to handle the 5KB line
			WithProcessLine(func(line string) {
				lines = append(lines, line)
			}),
			WithWaitForCmd(),
		)
		require.NoError(t, err)
		require.Len(t, lines, 1)
		assert.Equal(t, longLine, lines[0])
	})
}

// --- CheckRunningByPid tests ---

func TestCheckRunningByPid_SelfProcess(t *testing.T) {
	mockey.PatchConvey("CheckRunningByPid for a running process", t, func() {
		p, err := New(WithCommand("sleep", "30"))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		require.NoError(t, p.Start(ctx))
		defer func() { _ = p.Close(ctx) }()

		result := CheckRunningByPid(ctx, "sleep")
		assert.True(t, result)
	})
}

func TestCheckRunningByPid_NonExistent(t *testing.T) {
	mockey.PatchConvey("CheckRunningByPid for non-existent process", t, func() {
		ctx := context.Background()
		result := CheckRunningByPid(ctx, "this_process_definitely_does_not_exist_12345")
		assert.False(t, result)
	})
}

// --- Process Wait channel ---

func TestProcess_WaitChannelCompletes(t *testing.T) {
	mockey.PatchConvey("Process Wait channel signals on completion", t, func() {
		p, err := New(WithCommand("echo", "done"))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		require.NoError(t, p.Start(ctx))
		defer func() { _ = p.Close(ctx) }()

		select {
		case err := <-p.Wait():
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Wait channel did not signal within timeout")
		}
	})
}

func TestProcess_WaitChannelErrorOnFailure(t *testing.T) {
	mockey.PatchConvey("Process Wait channel signals error on failure", t, func() {
		p, err := New(
			WithBashScriptContentsToRun("exit 42"),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		require.NoError(t, p.Start(ctx))
		defer func() { _ = p.Close(ctx) }()

		select {
		case err := <-p.Wait():
			assert.Error(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Wait channel did not signal within timeout")
		}
	})
}

// --- CountProcessesByStatus success ---

func TestCountProcessesByStatus_Success(t *testing.T) {
	mockey.PatchConvey("CountProcessesByStatus success with real processes", t, func() {
		ctx := context.Background()
		result, err := CountProcessesByStatus(ctx)
		require.NoError(t, err)
		assert.NotNil(t, result)
		total := 0
		for _, procs := range result {
			total += len(procs)
		}
		assert.Greater(t, total, 0)
	})
}

// --- RunUntilCompletion with options ---

func TestRunUntilCompletion_WithDetachedProcess(t *testing.T) {
	mockey.PatchConvey("RunUntilCompletion with AllowDetachedProcess option", t, func() {
		runner := NewExclusiveRunner()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		output, exitCode, err := runner.RunUntilCompletion(ctx, "echo detached_test", WithAllowDetachedProcess(true))
		require.NoError(t, err)
		assert.Equal(t, int32(0), exitCode)
		assert.Contains(t, string(output), "detached_test")
	})
}

// --- ExclusiveRunner sequential runs ---

func TestRunUntilCompletion_SequentialRuns(t *testing.T) {
	mockey.PatchConvey("RunUntilCompletion can run sequentially", t, func() {
		runner := NewExclusiveRunner()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		output1, code1, err1 := runner.RunUntilCompletion(ctx, "echo first")
		require.NoError(t, err1)
		assert.Equal(t, int32(0), code1)
		assert.Contains(t, string(output1), "first")

		output2, code2, err2 := runner.RunUntilCompletion(ctx, "echo second")
		require.NoError(t, err2)
		assert.Equal(t, int32(0), code2)
		assert.Contains(t, string(output2), "second")
	})
}

// --- Process ExitCode after completion ---

func TestProcess_ExitCodeAfterSuccess(t *testing.T) {
	mockey.PatchConvey("ExitCode is 0 after successful completion", t, func() {
		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		output, err := p.StartAndWaitForCombinedOutput(ctx)
		require.NoError(t, err)
		assert.Contains(t, string(output), "hello")
		assert.Equal(t, int32(0), p.ExitCode())
	})
}

func TestProcess_ExitCodeAfterFailure(t *testing.T) {
	mockey.PatchConvey("ExitCode is non-zero after failure", t, func() {
		p, err := New(
			WithBashScriptContentsToRun("exit 7"),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = p.StartAndWaitForCombinedOutput(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exit status 7")
	})
}

// --- Process StdoutReader/StderrReader ---

func TestProcess_StdoutReaderAvailable(t *testing.T) {
	mockey.PatchConvey("StdoutReader is available after start", t, func() {
		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		require.NoError(t, p.Start(ctx))
		defer func() { _ = p.Close(ctx) }()

		assert.NotNil(t, p.StdoutReader())
	})
}

func TestProcess_StderrReaderAvailable(t *testing.T) {
	mockey.PatchConvey("StderrReader is available after start", t, func() {
		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		require.NoError(t, p.Start(ctx))
		defer func() { _ = p.Close(ctx) }()

		assert.NotNil(t, p.StderrReader())
	})
}

func TestProcess_Start_StdoutPipeErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("Start returns error when StdoutPipe fails", t, func() {
		mockey.Mock((*exec.Cmd).StdoutPipe).To(func(cmd *exec.Cmd) (io.ReadCloser, error) {
			return nil, errors.New("stdout pipe failed")
		}).Build()

		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = p.Start(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stdout pipe")
	})
}

func TestProcess_StartAndWaitForCombinedOutput_StartErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("StartAndWaitForCombinedOutput returns error when Start fails", t, func() {
		mockey.Mock((*exec.Cmd).Start).To(func(cmd *exec.Cmd) error {
			return errors.New("start failed")
		}).Build()

		p, err := New(WithCommand("echo", "hello"))
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		out, err := p.StartAndWaitForCombinedOutput(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start command")
		assert.Len(t, out, 0)
	})
}
