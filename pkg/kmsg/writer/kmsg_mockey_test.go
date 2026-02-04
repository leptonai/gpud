package writer

import (
	"bytes"
	"errors"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"
)

func TestNewWriter_OpenError_WithMockey(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping non-Linux platform")
	}

	mockey.PatchConvey("NewWriter returns noOpWriter on open error", t, func() {
		mockey.Mock(os.Geteuid).Return(0).Build()
		mockey.Mock(openKmsgForWrite).To(func(devFile string) (io.Writer, error) {
			return nil, errors.New("open failed")
		}).Build()

		wr := NewWriter("/dev/fake")
		_, ok := wr.(*noOpWriter)
		require.True(t, ok)
	})
}

func TestNewWriter_OpenSuccess_WithMockey(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping non-Linux platform")
	}

	mockey.PatchConvey("NewWriter returns kmsgWriter on success", t, func() {
		mockey.Mock(os.Geteuid).Return(0).Build()
		mockey.Mock(openKmsgForWrite).To(func(devFile string) (io.Writer, error) {
			return &bytes.Buffer{}, nil
		}).Build()

		wr := NewWriter("/dev/fake")
		_, ok := wr.(*kmsgWriter)
		require.True(t, ok)
	})
}
