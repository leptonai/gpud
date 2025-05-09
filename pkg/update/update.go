// Package update provides the update functionality for the server.
package update

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leptonai/gpud/pkg/log"
)

const DefaultUpdateURL = "https://pkg.gpud.dev/"

func Update(ver, url string) error {
	return update(ver, url, true, true)
}

// Updates the gpud binary by only downloading the tarball and unpacking it,
// without restarting the service or requiring root.
func UpdateOnlyBinary(ver, url string) error {
	return update(ver, url, false, false)
}

func update(ver, url string, requireRoot bool, useSystemd bool) error {
	log.Logger.Infow("starting gpud update", "version", ver, "url", url, "requireRoot", requireRoot, "useSystemd", useSystemd)

	if requireRoot {
		if err := RequireRoot(); err != nil {
			log.Logger.Errorf("this command needs to be run as root: %v", err)
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	dlPath, err := downloadLinuxTarball(ctx, ver, url)
	cancel()
	if err != nil {
		return err
	}
	log.Logger.Infow("downloaded update tarball", "path", dlPath)

	gpudPath, err := os.Executable()
	if err != nil {
		return err
	}
	if err := unpackLinuxTarball(dlPath, gpudPath); err != nil {
		return err
	}
	log.Logger.Infow("unpacked update tarball", "path", dlPath)

	if err := os.Remove(dlPath); err != nil {
		log.Logger.Errorw("failed to cleanup the downloaded update tarball", "error", err)
	}

	if useSystemd {
		if err := RestartGPUdSystemdUnit(); err != nil {
			if strings.Contains(err.Error(), "signal: terminated") {
				// an expected error
				log.Logger.Infof("gpud binary updated successfully. Waiting complete of systemd restart.")
			} else if errors.Is(err, errors.ErrUnsupported) {
				log.Logger.Errorf("gpud binary updated successfully. Please restart gpud to finish the update.")
			} else {
				log.Logger.Errorf("gpud binary updated successfully, but failed to restart gpud: %s. Please restart gpud to finish the update.", err)
			}
		} else {
			log.Logger.Infow("completed gpud update", "version", ver)
		}
	}

	return nil
}

func PackageUpdate(targetPackage, ver, baseUrl string) error {
	dlDir, err := os.UserCacheDir()
	if err != nil {
		dlDir = os.TempDir()
	}
	return packageUpdate(dlDir, targetPackage, ver, baseUrl)
}

func packageUpdate(dlDir string, targetPackage string, ver string, baseUrl string) error {
	if err := os.MkdirAll(dlDir, 0700); err != nil {
		return err
	}

	dlPath := filepath.Join(dlDir, targetPackage+ver)
	downloadUrl, err := url.JoinPath(baseUrl, "packages", targetPackage, ver)
	if err != nil {
		return err
	}
	err = downloadFile(downloadUrl, dlPath)
	if err != nil {
		return err
	}
	defer os.Remove(dlPath)

	if err = copyFile(dlPath, fmt.Sprintf("/var/lib/gpud/packages/%s/init.sh", targetPackage)); err != nil {
		return err
	}
	return nil
}
