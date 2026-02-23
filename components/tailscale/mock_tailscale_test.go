package tailscale

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"

	"github.com/leptonai/gpud/components"
	pkgfile "github.com/leptonai/gpud/pkg/file"
	"github.com/leptonai/gpud/pkg/systemd"
)

func TestParseTailscaleVersion(t *testing.T) {
	output := `1.80.0

	tailscale commit: abc
	other commit: def`
	version, err := parseTailscaleVersion(output)
	assert.NoError(t, err)
	assert.Equal(t, "1.80.0", version)
}

func TestParseTailscaleVersionInvalid(t *testing.T) {
	_, err := parseTailscaleVersion("no version here")
	assert.Error(t, err)
}

func TestCheckTailscaleInstalled_Found(t *testing.T) {
	mockey.PatchConvey("tailscale found", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			if name == "tailscale" {
				return "/usr/bin/tailscale", nil
			}
			return "", errors.New("not found")
		}).Build()

		result := checkTailscaleInstalled()
		assert.True(t, result)
	})
}

func TestCheckTailscaleInstalled_NotFound(t *testing.T) {
	mockey.PatchConvey("tailscale not found", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		result := checkTailscaleInstalled()
		assert.False(t, result)
	})
}

func TestCheckTailscaledInstalled_Found(t *testing.T) {
	mockey.PatchConvey("tailscaled found", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			if name == "tailscaled" {
				return "/usr/bin/tailscaled", nil
			}
			return "", errors.New("not found")
		}).Build()

		result := checkTailscaledInstalled()
		assert.True(t, result)
	})
}

func TestCheckTailscaledInstalled_NotFound(t *testing.T) {
	mockey.PatchConvey("tailscaled not found", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		result := checkTailscaledInstalled()
		assert.False(t, result)
	})
}

func TestGetTailscaleVersion_LocateError(t *testing.T) {
	mockey.PatchConvey("locate tailscale error", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		_, err := getTailscaleVersion()
		assert.Error(t, err)
	})
}

func TestCheckTailscaleInstalled_Public(t *testing.T) {
	mockey.PatchConvey("CheckTailscaleInstalled public", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			if name == "tailscale" {
				return "/usr/bin/tailscale", nil
			}
			return "", errors.New("not found")
		}).Build()

		result := CheckTailscaleInstalled()
		assert.True(t, result)
	})
}

func TestGetTailscaleVersion_SuccessWithMockey(t *testing.T) {
	mockey.PatchConvey("getTailscaleVersion success", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			return "/usr/bin/tailscale", nil
		}).Build()
		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("1.80.0\n"), nil
		}).Build()

		version, err := getTailscaleVersion()
		assert.NoError(t, err)
		assert.Equal(t, "1.80.0", version)
	})
}

func TestGetTailscaleVersion_CommandErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("getTailscaleVersion command error", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			return "/usr/bin/tailscale", nil
		}).Build()
		mockey.Mock((*exec.Cmd).CombinedOutput).To(func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("bad output"), errors.New("exec failed")
		}).Build()

		_, err := getTailscaleVersion()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tailscale --version failed")
	})
}

func TestComponentCheck_ServiceInactiveWithMockey(t *testing.T) {
	mockey.PatchConvey("component Check marks unhealthy when service inactive", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			return "/usr/bin/tailscaled", nil
		}).Build()
		mockey.Mock(systemd.IsActive).To(func(service string) (bool, error) {
			return false, nil
		}).Build()

		comp, err := New(&components.GPUdInstance{RootCtx: context.Background()})
		assert.NoError(t, err)
		result := comp.Check()
		assert.Equal(t, "tailscaled installed but tailscaled service is not active or failed to check", result.Summary())
	})
}
