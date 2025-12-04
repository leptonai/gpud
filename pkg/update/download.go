package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/release/distsign"
)

func downloadURLToFile(ctx context.Context, pathSrc, fileDst, pkgAddr string) (ret error) {
	logf := func(m string, args ...any) { log.Logger.Infof(m, args...) }
	c, err := distsign.NewClient(logf, pkgAddr)
	if err != nil {
		return err
	}
	return c.Download(ctx, pathSrc, fileDst)
}

func tarballName(ver, os, arch string) string {
	ubuntuVersion := detectUbuntuVersion()
	if ubuntuVersion == "" {
		return fmt.Sprintf("gpud_%s_%s_%s.tgz", ver, os, arch)
	}
	return fmt.Sprintf("gpud_%s_%s_%s_%s.tgz", ver, os, arch, ubuntuVersion)
}

func downloadLinuxTarball(ctx context.Context, ver, pkgAddr string) (string, error) {
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
	if err := downloadURLToFile(ctx, pkgsPath, dlPath, pkgAddr); err != nil {
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
	defer func() {
		_ = f.Close()
	}()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	return f.Close()
}

func unpackLinuxTarball(fileToUnpack string, fileToReplace string) error {
	f, err := os.Open(fileToUnpack)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() {
		_ = gr.Close()
	}()
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
			return fmt.Errorf("failed extracting %q: %w", fileToUnpack, err)
		}
		switch filepath.Base(th.Name) {
		case "gpud":
			files["gpud"]++
			if err := writeFile(tr, fileToReplace+".new", 0755); err != nil {
				return fmt.Errorf("failed extracting the new tailscale binary from %q: %w", fileToUnpack, err)
			}
		}
	}
	if !maps.Equal(files, wantFiles) {
		return fmt.Errorf("%q has missing or duplicate files: got %v, want %v", fileToUnpack, files, wantFiles)
	}

	// Only place the files in final locations after everything extracted correctly.
	if err := os.Rename(fileToReplace+".new", fileToReplace); err != nil {
		return err
	}
	log.Logger.Infof("Updated %s", fileToReplace)

	return nil
}

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = destinationFile.Close()
	}()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return err
	}

	err = destinationFile.Sync()
	return err
}
