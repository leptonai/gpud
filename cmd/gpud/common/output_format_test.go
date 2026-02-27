package common

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOutputFormat(t *testing.T) {
	t.Run("empty defaults to plain", func(t *testing.T) {
		got, err := ParseOutputFormat("")
		require.NoError(t, err)
		assert.Equal(t, OutputFormatPlain, got)
	})

	t.Run("whitespace-only defaults to plain", func(t *testing.T) {
		got, err := ParseOutputFormat("  \t  ")
		require.NoError(t, err)
		assert.Equal(t, OutputFormatPlain, got)
	})

	t.Run("plain accepted case-insensitively with surrounding whitespace", func(t *testing.T) {
		got, err := ParseOutputFormat("  PlAiN  ")
		require.NoError(t, err)
		assert.Equal(t, OutputFormatPlain, got)
	})

	t.Run("json accepted case-insensitively", func(t *testing.T) {
		got, err := ParseOutputFormat("JSON")
		require.NoError(t, err)
		assert.Equal(t, OutputFormatJSON, got)
	})

	t.Run("invalid rejected", func(t *testing.T) {
		_, err := ParseOutputFormat("yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid output format")
	})
}

func TestJSONCommandError(t *testing.T) {
	t.Run("constructor applies defaults", func(t *testing.T) {
		jerr := NewJSONCommandError("", "boom", 0)
		assert.Equal(t, "command_error", jerr.Code())
		assert.Equal(t, "boom", jerr.Error())
		assert.Equal(t, 1, jerr.ExitStatus())
		assert.Equal(t, JSONCommandErrorResponse{
			Error: JSONCommandErrorPayload{
				Code:    "command_error",
				Message: "boom",
			},
		}, jerr.Response())
	})

	t.Run("methods tolerate zero-value struct", func(t *testing.T) {
		jerr := &JSONCommandError{}
		assert.Equal(t, "", jerr.Error())
		assert.Equal(t, "command_error", jerr.Code())
		assert.Equal(t, 1, jerr.ExitStatus())
		assert.Equal(t, JSONCommandErrorResponse{
			Error: JSONCommandErrorPayload{
				Code:    "command_error",
				Message: "",
			},
		}, jerr.Response())
	})

	t.Run("methods tolerate nil receiver", func(t *testing.T) {
		var jerr *JSONCommandError
		assert.Equal(t, "", jerr.Error())
		assert.Equal(t, "command_error", jerr.Code())
		assert.Equal(t, 1, jerr.ExitStatus())
		assert.Equal(t, JSONCommandErrorResponse{
			Error: JSONCommandErrorPayload{
				Code:    "command_error",
				Message: "",
			},
		}, jerr.Response())
	})
}

func TestAsJSONCommandError(t *testing.T) {
	t.Run("returns false for non-JSON error", func(t *testing.T) {
		jerr, ok := AsJSONCommandError(errors.New("boom"))
		assert.False(t, ok)
		assert.Nil(t, jerr)
	})

	t.Run("unwraps wrapped JSON command errors", func(t *testing.T) {
		src := NewJSONCommandError("code", "boom", 7)
		jerr, ok := AsJSONCommandError(fmt.Errorf("context: %w", src))
		require.True(t, ok)
		assert.Same(t, src, jerr)
	})
}

func TestWrapOutputError(t *testing.T) {
	srcErr := errors.New("boom")

	t.Run("nil error returns nil", func(t *testing.T) {
		assert.NoError(t, WrapOutputError(OutputFormatJSON, "ignored", nil))
	})

	t.Run("plain returns original", func(t *testing.T) {
		wrapped := WrapOutputError(OutputFormatPlain, "test_error", srcErr)
		assert.ErrorIs(t, wrapped, srcErr)
	})

	t.Run("non-json format returns original", func(t *testing.T) {
		wrapped := WrapOutputError("yaml", "test_error", srcErr)
		assert.ErrorIs(t, wrapped, srcErr)
	})

	t.Run("json returns JSONCommandError", func(t *testing.T) {
		wrapped := WrapOutputError(OutputFormatJSON, "test_error", srcErr)
		require.Error(t, wrapped)

		jerr, ok := AsJSONCommandError(wrapped)
		require.True(t, ok)
		assert.Equal(t, "test_error", jerr.Code())
		assert.Equal(t, "boom", jerr.Error())
		assert.Equal(t, 1, jerr.ExitStatus())
	})

	t.Run("json preserves existing JSONCommandError exit status", func(t *testing.T) {
		src := NewJSONCommandError("original_code", "boom", 7)
		wrapped := WrapOutputError(OutputFormatJSON, "wrapped_code", src)

		jerr, ok := AsJSONCommandError(wrapped)
		require.True(t, ok)
		assert.Equal(t, "wrapped_code", jerr.Code())
		assert.Equal(t, 7, jerr.ExitStatus())
	})

	t.Run("json keeps existing code when wrapper code is empty", func(t *testing.T) {
		src := NewJSONCommandError("original_code", "boom", 9)
		wrapped := WrapOutputError(OutputFormatJSON, "", src)

		jerr, ok := AsJSONCommandError(wrapped)
		require.True(t, ok)
		assert.Equal(t, "original_code", jerr.Code())
		assert.Equal(t, 9, jerr.ExitStatus())
	})

	t.Run("json uses default code when wrapper code is empty", func(t *testing.T) {
		wrapped := WrapOutputError(OutputFormatJSON, "", srcErr)
		jerr, ok := AsJSONCommandError(wrapped)
		require.True(t, ok)
		assert.Equal(t, "command_error", jerr.Code())
		assert.Equal(t, 1, jerr.ExitStatus())
	})

	t.Run("json uses wrapped error message", func(t *testing.T) {
		src := NewJSONCommandError("code", "boom", 5)
		wrapped := WrapOutputError(OutputFormatJSON, "wrapped", fmt.Errorf("context: %w", src))

		jerr, ok := AsJSONCommandError(wrapped)
		require.True(t, ok)
		assert.Equal(t, "wrapped", jerr.Code())
		assert.Equal(t, 5, jerr.ExitStatus())
		assert.Equal(t, "context: boom", jerr.Error())
	})
}

func TestWriteJSONToWriter(t *testing.T) {
	t.Run("writes expected JSON", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, WriteJSONToWriter(&buf, map[string]string{"x": "y"}))
		assert.Equal(t, "{\"x\":\"y\"}\n", buf.String())
	})

	t.Run("does not escape HTML", func(t *testing.T) {
		var buf bytes.Buffer
		require.NoError(t, WriteJSONToWriter(&buf, map[string]string{"x": "<b>"}))
		assert.Equal(t, "{\"x\":\"<b>\"}\n", buf.String())
	})

	t.Run("fails for nil writer", func(t *testing.T) {
		err := WriteJSONToWriter(nil, map[string]string{"x": "y"})
		require.Error(t, err)
		assert.EqualError(t, err, "writer is nil")
	})

	t.Run("propagates writer errors", func(t *testing.T) {
		expectedErr := errors.New("write failed")
		err := WriteJSONToWriter(errWriter{err: expectedErr}, map[string]string{"x": "y"})
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestWriteJSON(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)

	oldStdout := os.Stdout
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
	})
	t.Cleanup(func() {
		_ = r.Close()
	})

	require.NoError(t, WriteJSON(map[string]string{"x": "y"}))
	require.NoError(t, w.Close())

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "{\"x\":\"y\"}\n", string(got))
}

type errWriter struct {
	err error
}

func (w errWriter) Write(p []byte) (int, error) {
	return 0, w.err
}
