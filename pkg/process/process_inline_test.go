package process

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Verifies that inline bash mode runs without creating a temp file.
func TestProcessWithRunBashInline(t *testing.T) {
	p, err := New(
		WithCommand("echo", "hello"),
		WithCommand("echo inline"),
		WithRunAsBashScript(),
		WithRunBashInline(),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-p.Wait():
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for process exit")
	}

	// Ensure no on-disk bash file was created
	if proc, ok := p.(*process); ok {
		if proc.runBashFile != nil {
			if _, err := os.Stat(proc.runBashFile.Name()); err == nil {
				t.Fatalf("expected no temp bash file to exist, but found: %s", proc.runBashFile.Name())
			}
		}
	}
}

func TestProcessWithRunBashInline_QuotesAndMeta(t *testing.T) {
	// Ensure tricky quoting works fine via stdin (-s), not -c.
	tmp, err := os.CreateTemp("", "process-inline-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	p, err := New(
		WithBashScriptContentsToRun(`echo 'hello "world"' && echo a|tr 'a' 'A'`),
		WithRunAsBashScript(),
		WithRunBashInline(),
		WithOutputFile(tmp),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-p.Wait():
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for process exit")
	}
	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "hello \"world\"") {
		t.Fatalf("expected quoted output, got: %q", out)
	}
	if !strings.Contains(out, "A") { // pipeline/tr substitution succeeded
		t.Fatalf("expected pipeline output with capital A, got: %q", out)
	}
}

func TestProcessWithRunBashInline_MultiCommand(t *testing.T) {
	// Use command list; runtime assembles bash program + headers and feeds stdin.
	tmp, err := os.CreateTemp("", "process-inline-*.log")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	p, err := New(
		WithCommand("echo", "hello world"),
		WithCommand("echo hello && echo 111 | grep 1"),
		WithRunAsBashScript(),
		WithRunBashInline(),
		WithOutputFile(tmp),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-p.Wait():
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for process exit")
	}
	if err := p.Close(ctx); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, "hello world") {
		t.Fatalf("missing 'hello world' in output: %q", out)
	}
	if !strings.Contains(out, "111") {
		t.Fatalf("missing '111' in output: %q", out)
	}
}
