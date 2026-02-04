package process

import (
	"errors"
	"os"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_BashFileWriteErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("New with bash file write error", t, func() {
		f, err := os.CreateTemp("", "bashfile")
		require.NoError(t, err)
		defer func() {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}()

		mockey.Mock(os.CreateTemp).To(func(dir, pattern string) (*os.File, error) {
			return f, nil
		}).Build()
		mockey.Mock((*os.File).Write).To(func(_ *os.File, _ []byte) (int, error) {
			return 0, errors.New("write failed")
		}).Build()

		_, err = New(
			WithRunAsBashScript(),
			WithCommand("echo", "hello"),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write failed")
	})
}
