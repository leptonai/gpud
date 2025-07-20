// Package update provides the update functionality for the server.
package update

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/osutil"
)

const DefaultUpdateURL = "https://pkg.gpud.dev/"

// UpdateExecutable updates the GPUd binary executable itself.
func UpdateExecutable(ver, url string, requireRoot bool) error {
	log.Logger.Infow("starting gpud update", "version", ver, "url", url, "requireRoot", requireRoot)

	if requireRoot {
		if err := osutil.RequireRoot(); err != nil {
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
