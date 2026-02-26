//go:build linux

package file

import (
	"errors"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckFDLimitSupported_StatErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("CheckFDLimitSupported returns false on stat error", t, func() {
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return nil, errors.New("stat failed")
		}).Build()

		assert.False(t, CheckFDLimitSupported())
	})
}

func TestCheckFileHandlesSupported_StatErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("CheckFileHandlesSupported returns false on stat error", t, func() {
		mockey.Mock(os.Stat).To(func(name string) (os.FileInfo, error) {
			return nil, errors.New("stat failed")
		}).Build()

		assert.False(t, CheckFileHandlesSupported())
	})
}

func TestGetUsage_AllProcsErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("GetUsage returns error when AllProcs fails", t, func() {
		mockey.Mock(procfs.AllProcs).To(func() (procfs.Procs, error) {
			return nil, errors.New("allprocs failed")
		}).Build()

		_, err := GetUsage()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "allprocs failed")
	})
}

func TestGetUsage_SkipNotExistErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("GetUsage skips not-exist errors", t, func() {
		mockey.Mock(procfs.AllProcs).To(func() (procfs.Procs, error) {
			return procfs.Procs{{}}, nil
		}).Build()
		mockey.Mock((*procfs.Proc).FileDescriptorsLen).To(func(_ *procfs.Proc) (int, error) {
			return 0, os.ErrNotExist
		}).Build()

		usage, err := GetUsage()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), usage)
	})
}
