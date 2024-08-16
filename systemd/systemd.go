package systemd

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
)

//go:embed gpud.service
var GPUDService string

//go:embed gpud.logrotate.conf
var GPUDLogrotate string

const (
	DefaultEnvFile       = "/etc/default/gpud"
	DefaultUnitFile      = "/etc/systemd/system/gpud.service"
	DefaultLogrotateConf = "/etc/logrotate.d/gpud"
	DefaultBinPath       = "/usr/sbin/gpud"
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

func LogrotateInit() error {
	if _, err := os.Stat(DefaultLogrotateConf); os.IsNotExist(err) {
		return writeConfigFile()
	}
	content, err := os.ReadFile(DefaultLogrotateConf)
	if err != nil {
		return fmt.Errorf("failed to read logrotate config file: %w", err)
	}
	if strings.TrimSpace(string(content)) != strings.TrimSpace(GPUDLogrotate) {
		return writeConfigFile()
	}
	return nil
}

func writeConfigFile() error {
	if err := os.WriteFile(DefaultLogrotateConf, []byte(GPUDLogrotate), 0644); err != nil {
		return fmt.Errorf("failed to write logrotate config file: %w", err)
	}
	return nil
}
