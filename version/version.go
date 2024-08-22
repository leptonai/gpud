// Package version provides the version information for the gpud server.
package version

import "runtime"

var (
	// Package is filled at linking time
	Package = "github.com/leptonai/gpud"

	// Version holds the complete version number. Filled in at linking time.
	Version = "0.0.1+unknown"

	// Revision is filled with the VCS (e.g. git) revision being used to build
	// the program at linking time.
	Revision = ""

	// BuildTimestamp is the build timestamp.
	BuildTimestamp = ""

	// GoVersion is Go tree's version.
	GoVersion = runtime.Version()
)
