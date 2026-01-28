package containerd

import (
	"errors"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"

	pkgfile "github.com/leptonai/gpud/pkg/file"
)

func TestCheckContainerdInstalled_WithMockey(t *testing.T) {
	mockey.PatchConvey("checkContainerdInstalled respects LocateExecutable", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			if name == "containerd" {
				return "/usr/bin/containerd", nil
			}
			return "", errors.New("not found")
		}).Build()

		assert.True(t, checkContainerdInstalled())
	})
}

func TestCheckContainerdInstalled_NotFoundWithMockey(t *testing.T) {
	mockey.PatchConvey("checkContainerdInstalled handles not found", t, func() {
		mockey.Mock(pkgfile.LocateExecutable).To(func(name string) (string, error) {
			return "", errors.New("not found")
		}).Build()

		assert.False(t, checkContainerdInstalled())
	})
}
