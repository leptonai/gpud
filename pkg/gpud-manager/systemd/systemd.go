// Package systemd provides the systemd artifacts and variables for the gpud server.
package systemd

import (
	_ "embed"
	"os"
)

//go:embed gpud.service
var GPUDService string

const (
	DefaultEnvFile  = "/etc/default/gpud"
	DefaultUnitFile = "/etc/systemd/system/gpud.service"
	DefaultBinPath  = "/usr/sbin/gpud"
)

func DefaultBinExists() bool {
	_, err := os.Stat(DefaultBinPath)
	return err == nil
}

func CreateDefaultEnvFile() error {
	if _, err := os.Stat(DefaultEnvFile); err == nil { // to not overwrite
		return nil
	}

	f, err := os.OpenFile(DefaultEnvFile, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(`# gpud environment variables are set here
FLAGS="--log-level=info"
`)
	return err
}
