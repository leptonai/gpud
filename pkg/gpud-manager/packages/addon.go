package packages

import (
	"errors"
	"os"
	"path/filepath"
)

// InstallAddonRequest is the request to install an addon.
type InstallAddonRequest struct {
	// PackagesDir is the directory where all the packages are installed.
	// And the addon script will be installed under the directory named after
	// the name of the addon.
	// e.g., "/var/lib/gpud/packages".
	PackagesDir string `json:"dir"`

	// Name is the name of the addon to install.
	Name string `json:"name"`

	// Script is the script to install the addon.
	Script string `json:"script"`
}

var (
	ErrPackagesDirRequired  = errors.New("packages dir is required")
	ErrPackagesDirInvalid   = errors.New("packages dir is invalid")
	ErrPackagesDirNotExists = errors.New("packages dir does not exist")
	ErrNameRequired         = errors.New("name is required")
	ErrScriptRequired       = errors.New("script is required")
)

func (r *InstallAddonRequest) Validate() error {
	if r.PackagesDir == "" {
		return ErrPackagesDirRequired
	}
	if !filepath.IsAbs(r.PackagesDir) {
		return ErrPackagesDirInvalid
	}
	if _, err := os.Stat(r.PackagesDir); os.IsNotExist(err) {
		return ErrPackagesDirNotExists
	}

	if r.Name == "" {
		return ErrNameRequired
	}

	if r.Script == "" {
		return ErrScriptRequired
	}

	return nil
}

func (r *InstallAddonRequest) Install() error {
	if err := r.Validate(); err != nil {
		return err
	}

	addonDir := filepath.Join(r.PackagesDir, r.Name)
	if err := os.MkdirAll(addonDir, 0755); err != nil {
		return err
	}

	addonScript := filepath.Join(addonDir, "init.sh")
	if err := os.WriteFile(addonScript, []byte(r.Script), 0755); err != nil {
		return err
	}

	return nil
}
