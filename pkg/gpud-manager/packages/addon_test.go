package packages

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAddonRequest_Validate(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "addon-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		request *InstallAddonRequest
		wantErr error
	}{
		{
			name: "empty packages dir",
			request: &InstallAddonRequest{
				PackagesDir: "",
				Name:        "test-addon",
				Script:      "#!/bin/bash\necho 'test'",
			},
			wantErr: ErrPackagesDirRequired,
		},
		{
			name: "relative packages dir",
			request: &InstallAddonRequest{
				PackagesDir: "relative/path",
				Name:        "test-addon",
				Script:      "#!/bin/bash\necho 'test'",
			},
			wantErr: ErrPackagesDirInvalid,
		},
		{
			name: "non-existent packages dir",
			request: &InstallAddonRequest{
				PackagesDir: "/path/that/does/not/exist",
				Name:        "test-addon",
				Script:      "#!/bin/bash\necho 'test'",
			},
			wantErr: ErrPackagesDirNotExists,
		},
		{
			name: "empty name",
			request: &InstallAddonRequest{
				PackagesDir: tmpDir,
				Name:        "",
				Script:      "#!/bin/bash\necho 'test'",
			},
			wantErr: ErrNameRequired,
		},
		{
			name: "empty script",
			request: &InstallAddonRequest{
				PackagesDir: tmpDir,
				Name:        "test-addon",
				Script:      "",
			},
			wantErr: ErrScriptRequired,
		},
		{
			name: "valid request",
			request: &InstallAddonRequest{
				PackagesDir: tmpDir,
				Name:        "test-addon",
				Script:      "#!/bin/bash\necho 'test'",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInstallAddonRequest_Install(t *testing.T) {
	t.Run("invalid request", func(t *testing.T) {
		req := &InstallAddonRequest{
			PackagesDir: "",
			Name:        "test-addon",
			Script:      "#!/bin/bash\necho 'test'",
		}

		err := req.Install()
		if err != ErrPackagesDirRequired {
			t.Errorf("Install() error = %v, wantErr %v", err, ErrPackagesDirRequired)
		}
	})

	t.Run("successful installation", func(t *testing.T) {
		// Create a temporary directory for testing
		tmpDir, err := os.MkdirTemp("", "addon-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		testScript := "#!/bin/bash\necho 'Hello from addon'"
		req := &InstallAddonRequest{
			PackagesDir: tmpDir,
			Name:        "test-addon",
			Script:      testScript,
		}

		// Install the addon
		err = req.Install()
		if err != nil {
			t.Fatalf("Install() error = %v", err)
		}

		// Verify the addon directory was created
		addonDir := filepath.Join(tmpDir, "test-addon")
		if _, err := os.Stat(addonDir); os.IsNotExist(err) {
			t.Errorf("Addon directory was not created: %s", addonDir)
		}

		// Verify the script file was created with correct content
		scriptPath := filepath.Join(addonDir, "init.sh")
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			t.Errorf("Failed to read script file: %v", err)
		}

		if string(content) != testScript {
			t.Errorf("Script content mismatch. Got: %s, Want: %s", string(content), testScript)
		}

		// Verify the script file has executable permissions
		fileInfo, err := os.Stat(scriptPath)
		if err != nil {
			t.Errorf("Failed to stat script file: %v", err)
		}

		expectedMode := os.FileMode(0755)
		if fileInfo.Mode().Perm() != expectedMode {
			t.Errorf("Script file permissions mismatch. Got: %v, Want: %v", fileInfo.Mode().Perm(), expectedMode)
		}
	})

	t.Run("directory creation failure", func(t *testing.T) {
		// Create a file where we expect a directory to test error handling
		tmpDir, err := os.MkdirTemp("", "addon-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// Create a file with the addon name to cause directory creation to fail
		conflictPath := filepath.Join(tmpDir, "test-addon")
		if err := os.WriteFile(conflictPath, []byte("conflict"), 0644); err != nil {
			t.Fatalf("Failed to create conflict file: %v", err)
		}

		req := &InstallAddonRequest{
			PackagesDir: tmpDir,
			Name:        "test-addon",
			Script:      "#!/bin/bash\necho 'test'",
		}

		// Install should fail due to file conflict
		err = req.Install()
		if err == nil {
			t.Error("Install() should have failed due to file conflict")
		}
	})
}
