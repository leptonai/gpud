package common

import (
	"github.com/urfave/cli"

	pkgconfig "github.com/leptonai/gpud/pkg/config"
)

// ResolveDataDir resolves the data directory using the --data-dir flag when provided,
// otherwise it falls back to the default GPUd data directory selection logic.
func ResolveDataDir(cliContext *cli.Context) (string, error) {
	if cliContext != nil && cliContext.IsSet("data-dir") {
		return pkgconfig.ResolveDataDir(cliContext.String("data-dir"))
	}
	return pkgconfig.ResolveDataDir("")
}

// StateFileFromContext returns the state file path derived from the resolved data directory.
func StateFileFromContext(cliContext *cli.Context) (string, error) {
	dataDir, err := ResolveDataDir(cliContext)
	if err != nil {
		return "", err
	}
	return pkgconfig.StateFilePath(dataDir), nil
}

// VersionFileFromContext returns the version file path, honoring user overrides.
func VersionFileFromContext(cliContext *cli.Context, flagValue string, flagSet bool) (string, error) {
	dataDir, err := ResolveDataDir(cliContext)
	if err != nil {
		return "", err
	}
	if flagSet {
		return flagValue, nil
	}
	return pkgconfig.VersionFilePath(dataDir), nil
}
