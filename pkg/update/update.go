// Package update provides the update functionality for the server.
package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/third_party/tailscale/distsign"
)

func RequireRoot() error {
	if os.Geteuid() == 0 {
		return nil
	}
	return errors.New("this command needs to be run as root")
}

func downloadURLToFile(pathSrc, fileDst, pkgAddr string) (ret error) {
	logf := func(m string, args ...any) { log.Logger.Infof(m, args...) }
	c, err := distsign.NewClient(logf, pkgAddr)
	if err != nil {
		return err
	}
	return c.Download(context.Background(), pathSrc, fileDst)
}

func detectUbuntuVersion() string {
	outputBytes, err := exec.Command("lsb_release", "-i", "-s").Output()
	if err != nil {
		return ""
	}
	osName := strings.TrimSpace(strings.ToLower(string(outputBytes)))
	if osName != "ubuntu" {
		return ""
	}
	outputBytes, err = exec.Command("lsb_release", "-r", "-s").Output()
	if err != nil {
		return ""
	}
	version := strings.TrimSpace(string(outputBytes))
	if version == "20.04" || version == "22.04" || version == "24.04" {
		return "ubuntu" + version
	}
	return ""
}

func tarballName(ver, os, arch string) string {
	ubuntuVersion := detectUbuntuVersion()
	if ubuntuVersion == "" {
		return fmt.Sprintf("gpud_%s_%s_%s.tgz", ver, os, arch)
	}
	return fmt.Sprintf("gpud_%s_%s_%s_%s.tgz", ver, os, arch, ubuntuVersion)
}

func downloadLinuxTarball(ver, pkgAddr string) (string, error) {
	dlDir, err := os.UserCacheDir()
	if err != nil {
		dlDir = os.TempDir()
	}
	dlDir = filepath.Join(dlDir, "gpud-update")
	if err := os.MkdirAll(dlDir, 0700); err != nil {
		return "", err
	}
	pkgsPath := tarballName(ver, runtime.GOOS, runtime.GOARCH)
	dlPath := filepath.Join(dlDir, path.Base(pkgsPath))
	if err := downloadURLToFile(pkgsPath, dlPath, pkgAddr); err != nil {
		return "", err
	}
	return dlPath, nil
}

func writeFile(r io.Reader, path string, perm os.FileMode) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing file at %q: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	return f.Close()
}

func unpackLinuxTarball(path string) error {
	gpudPath, err := os.Executable()
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	files := make(map[string]int)
	wantFiles := map[string]int{
		"gpud": 1,
	}
	for {
		th, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed extracting %q: %w", path, err)
		}
		switch filepath.Base(th.Name) {
		case "gpud":
			files["gpud"]++
			if err := writeFile(tr, gpudPath+".new", 0755); err != nil {
				return fmt.Errorf("failed extracting the new tailscale binary from %q: %w", path, err)
			}
		}
	}
	if !maps.Equal(files, wantFiles) {
		return fmt.Errorf("%q has missing or duplicate files: got %v, want %v", path, files, wantFiles)
	}

	// Only place the files in final locations after everything extracted correctly.
	if err := os.Rename(gpudPath+".new", gpudPath); err != nil {
		return err
	}
	log.Logger.Infof("Updated %s", gpudPath)

	return nil
}

func SystemctlExists() error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		// Likely not a systemd-managed distro.
		return errors.ErrUnsupported
	}
	return nil
}

func EnableSystemdUnit() error {
	if err := SystemctlExists(); err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "enable", "gpud.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl enable failed: %w output: %s", err, out)
	}
	return nil
}

func DisableSystemdUnit() error {
	if err := SystemctlExists(); err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "disable", "gpud.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl disable failed: %w output: %s", err, out)
	}
	return nil
}

func RestartSystemdUnit() error {
	if err := SystemctlExists(); err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %w output: %s", err, out)
	}
	if out, err := exec.Command("systemctl", "restart", "gpud.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl restart failed: %w output: %s", err, out)
	}
	return nil
}

func StopSystemdUnit() error {
	if err := SystemctlExists(); err != nil {
		return err
	}
	if out, err := exec.Command("systemctl", "stop", "gpud.service").CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl stop failed: %w output: %s", err, out)
	}
	return nil
}

func Update(ver, url string) error {
	if err := RequireRoot(); err != nil {
		log.Logger.Errorf("this command needs to be run as root: %v", err)
		return err
	}

	dlPath, err := downloadLinuxTarball(ver, url)
	if err != nil {
		return err
	}
	fmt.Printf("Extracting %q", dlPath)
	if err := unpackLinuxTarball(dlPath); err != nil {
		return err
	}
	if err := os.Remove(dlPath); err != nil {
		log.Logger.Errorf("failed to cleanup: %s", err)
	}
	if err := RestartSystemdUnit(); err != nil {
		if strings.Contains(err.Error(), "signal: terminated") {
			// an expected error
			log.Logger.Infof("gpud binary updated successfully. Waiting complete of systemd restart.")
		} else if errors.Is(err, errors.ErrUnsupported) {
			log.Logger.Errorf("gpud binary updated successfully. Please restart gpud to finish the update.")
		} else {
			log.Logger.Errorf("gpud binary updated successfully, but failed to restart gpud: %s. Please restart gpud to finish the update.", err)
		}
	} else {
		log.Logger.Infof("updating gpud to version %s completed", ver)
	}

	return nil
}
