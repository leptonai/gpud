// Package schema provides the schema for the process state.
package schema

// Row represents a row in the process state table.
type Row struct {
	ScriptHash             string  `json:"script_hash"`
	LastStartedUnixSeconds int64   `json:"last_started_unix_seconds"`
	ScriptName             *string `json:"script_name,omitempty"`
	LastExitCode           *int    `json:"last_exit_code,omitempty"`
	LastOutput             *string `json:"last_output,omitempty"`
}
