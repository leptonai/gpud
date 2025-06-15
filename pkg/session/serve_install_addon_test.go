package session

import (
	"os"
	"path/filepath"
	"testing"

	pkdpackages "github.com/leptonai/gpud/pkg/gpud-manager/packages"
)

func TestSession_processInstallAddon(t *testing.T) {
	// Create a minimal Session for testing
	s := &Session{}

	t.Run("nil request", func(t *testing.T) {
		resp := &Response{}
		s.processInstallAddon(nil, resp)

		expectedError := "install addon request is nil"
		if resp.Error != expectedError {
			t.Errorf("processInstallAddon() error = %q, want %q", resp.Error, expectedError)
		}
	})

	t.Run("request with empty packages dir (validation failure)", func(t *testing.T) {
		req := &pkdpackages.InstallAddonRequest{
			PackagesDir: "",
			Name:        "test-addon",
			Script:      "#!/bin/bash\necho 'test'",
		}
		resp := &Response{}

		s.processInstallAddon(req, resp)

		if resp.Error != pkdpackages.ErrPackagesDirRequired.Error() {
			t.Errorf("processInstallAddon() error = %q, want %q", resp.Error, pkdpackages.ErrPackagesDirRequired.Error())
		}
	})

	t.Run("request with relative packages dir (validation failure)", func(t *testing.T) {
		req := &pkdpackages.InstallAddonRequest{
			PackagesDir: "relative/path",
			Name:        "test-addon",
			Script:      "#!/bin/bash\necho 'test'",
		}
		resp := &Response{}

		s.processInstallAddon(req, resp)

		if resp.Error != pkdpackages.ErrPackagesDirInvalid.Error() {
			t.Errorf("processInstallAddon() error = %q, want %q", resp.Error, pkdpackages.ErrPackagesDirInvalid.Error())
		}
	})

	t.Run("request with non-existent packages dir (validation failure)", func(t *testing.T) {
		req := &pkdpackages.InstallAddonRequest{
			PackagesDir: "/path/that/does/not/exist",
			Name:        "test-addon",
			Script:      "#!/bin/bash\necho 'test'",
		}
		resp := &Response{}

		s.processInstallAddon(req, resp)

		if resp.Error != pkdpackages.ErrPackagesDirNotExists.Error() {
			t.Errorf("processInstallAddon() error = %q, want %q", resp.Error, pkdpackages.ErrPackagesDirNotExists.Error())
		}
	})

	t.Run("request with empty name (validation failure)", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "serve-addon-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		req := &pkdpackages.InstallAddonRequest{
			PackagesDir: tmpDir,
			Name:        "",
			Script:      "#!/bin/bash\necho 'test'",
		}
		resp := &Response{}

		s.processInstallAddon(req, resp)

		if resp.Error != pkdpackages.ErrNameRequired.Error() {
			t.Errorf("processInstallAddon() error = %q, want %q", resp.Error, pkdpackages.ErrNameRequired.Error())
		}
	})

	t.Run("request with empty script (validation failure)", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "serve-addon-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		req := &pkdpackages.InstallAddonRequest{
			PackagesDir: tmpDir,
			Name:        "test-addon",
			Script:      "",
		}
		resp := &Response{}

		s.processInstallAddon(req, resp)

		if resp.Error != pkdpackages.ErrScriptRequired.Error() {
			t.Errorf("processInstallAddon() error = %q, want %q", resp.Error, pkdpackages.ErrScriptRequired.Error())
		}
	})

	t.Run("successful installation", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "serve-addon-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		testScript := "#!/bin/bash\necho 'Hello from addon'"
		req := &pkdpackages.InstallAddonRequest{
			PackagesDir: tmpDir,
			Name:        "test-addon",
			Script:      testScript,
		}
		resp := &Response{}

		s.processInstallAddon(req, resp)

		// Should have no error
		if resp.Error != "" {
			t.Errorf("processInstallAddon() error = %q, want empty", resp.Error)
		}

		// Verify the addon was actually installed
		addonDir := filepath.Join(tmpDir, "test-addon")
		if _, err := os.Stat(addonDir); os.IsNotExist(err) {
			t.Errorf("Addon directory was not created: %s", addonDir)
		}

		scriptPath := filepath.Join(addonDir, "init.sh")
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Errorf("Failed to read script file: %v", err)
		}

		if string(content) != testScript {
			t.Errorf("Script content mismatch. Got: %s, Want: %s", string(content), testScript)
		}
	})

	t.Run("installation with directory creation failure", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "serve-addon-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Create a file with the addon name to cause directory creation to fail
		conflictPath := filepath.Join(tmpDir, "test-addon")
		if err := os.WriteFile(conflictPath, []byte("conflict"), 0644); err != nil {
			t.Fatalf("Failed to create conflict file: %v", err)
		}

		req := &pkdpackages.InstallAddonRequest{
			PackagesDir: tmpDir,
			Name:        "test-addon",
			Script:      "#!/bin/bash\necho 'test'",
		}
		resp := &Response{}

		s.processInstallAddon(req, resp)

		// Should have an error
		if resp.Error == "" {
			t.Error("processInstallAddon() should have failed due to file conflict")
		}
	})
}
