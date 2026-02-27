package memory

import (
	"errors"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCurrentBPFJITBufferBytes_SuccessWithMockey(t *testing.T) {
	mockey.PatchConvey("returns bytes from reader", t, func() {
		const expected uint64 = 4096

		mockey.Mock(isLinux).To(func() bool {
			return true
		}).Build()
		mockey.Mock(readBPFJITBufferBytes).To(func(_ string) (uint64, error) {
			return expected, nil
		}).Build()

		got, err := getCurrentBPFJITBufferBytes()
		require.NoError(t, err)
		assert.Equal(t, expected, got)
	})
}

func TestGetCurrentBPFJITBufferBytes_NonRootReadErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("returns nil error for non-root read failures", t, func() {
		readErr := errors.New("permission denied")

		mockey.Mock(isLinux).To(func() bool {
			return true
		}).Build()
		mockey.Mock(readBPFJITBufferBytes).To(func(_ string) (uint64, error) {
			return 0, readErr
		}).Build()
		mockey.Mock(os.Geteuid).To(func() int {
			return 1000
		}).Build()

		got, err := getCurrentBPFJITBufferBytes()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), got)
	})
}

func TestGetCurrentBPFJITBufferBytes_RootReadErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("returns read error for root", t, func() {
		readErr := errors.New("read failed")

		mockey.Mock(isLinux).To(func() bool {
			return true
		}).Build()
		mockey.Mock(readBPFJITBufferBytes).To(func(_ string) (uint64, error) {
			return 0, readErr
		}).Build()
		mockey.Mock(os.Geteuid).To(func() int {
			return 0
		}).Build()

		got, err := getCurrentBPFJITBufferBytes()
		require.Error(t, err)
		require.ErrorIs(t, err, readErr)
		assert.Equal(t, uint64(0), got)
	})
}

func TestGetCurrentBPFJITBufferBytes_NonLinuxWithMockey(t *testing.T) {
	mockey.PatchConvey("returns zero on non-linux", t, func() {
		mockey.Mock(isLinux).To(func() bool {
			return false
		}).Build()

		got, err := getCurrentBPFJITBufferBytes()
		require.NoError(t, err)
		assert.Equal(t, uint64(0), got)
	})
}
