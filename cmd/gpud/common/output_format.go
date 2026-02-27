package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	OutputFormatPlain = "plain"
	OutputFormatJSON  = "json"
)

// ParseOutputFormat validates and normalizes output format values.
// Empty values default to plain output.
func ParseOutputFormat(raw string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	if normalized == "" {
		return OutputFormatPlain, nil
	}

	switch normalized {
	case OutputFormatPlain, OutputFormatJSON:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid output format %q (supported: %q, %q)", raw, OutputFormatPlain, OutputFormatJSON)
	}
}

type JSONCommandError struct {
	code     string
	message  string
	exitCode int
}

type JSONCommandErrorResponse struct {
	Error JSONCommandErrorPayload `json:"error"`
}

type JSONCommandErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewJSONCommandError(code string, message string, exitCode int) *JSONCommandError {
	if code == "" {
		code = "command_error"
	}
	if exitCode == 0 {
		exitCode = 1
	}
	return &JSONCommandError{
		code:     code,
		message:  message,
		exitCode: exitCode,
	}
}

func (e *JSONCommandError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *JSONCommandError) ExitStatus() int {
	if e == nil || e.exitCode == 0 {
		return 1
	}
	return e.exitCode
}

func (e *JSONCommandError) Code() string {
	if e == nil || e.code == "" {
		return "command_error"
	}
	return e.code
}

func (e *JSONCommandError) Response() JSONCommandErrorResponse {
	if e == nil {
		return JSONCommandErrorResponse{
			Error: JSONCommandErrorPayload{
				Code:    "command_error",
				Message: "",
			},
		}
	}
	return JSONCommandErrorResponse{
		Error: JSONCommandErrorPayload{
			Code:    e.Code(),
			Message: e.message,
		},
	}
}

func AsJSONCommandError(err error) (*JSONCommandError, bool) {
	var jerr *JSONCommandError
	if !errors.As(err, &jerr) {
		return nil, false
	}
	return jerr, true
}

func WrapOutputError(outputFormat, code string, err error) error {
	if err == nil {
		return nil
	}
	if outputFormat != OutputFormatJSON {
		return err
	}
	if jerr, ok := AsJSONCommandError(err); ok {
		if strings.TrimSpace(code) == "" {
			code = jerr.Code()
		}
		return NewJSONCommandError(code, err.Error(), jerr.ExitStatus())
	}
	return NewJSONCommandError(code, err.Error(), 1)
}

func WriteJSON(v any) error {
	return WriteJSONToWriter(os.Stdout, v)
}

func WriteJSONToWriter(w io.Writer, v any) error {
	if w == nil {
		return errors.New("writer is nil")
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
