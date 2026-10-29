package session

import (
	"errors"
	"fmt"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/require"

	gpudsystemd "github.com/leptonai/gpud/pkg/gpud-manager/systemd"
	"github.com/leptonai/gpud/pkg/systemd"
)

func TestUpdatePreservesExperimentalHealthValidation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			mockey.PatchConvey("auto-update retains collector mode", t, func() {
				mockey.Mock(systemd.IsActive).Return(true, nil).Build()
				captured := errors.New("captured environment")
				calls := 0
				mockey.Mock(gpudsystemd.CreateDefaultEnvFile).To(func(_, _ string, _ bool, actual bool, _ ...string) error {
					calls++
					require.Equal(t, enabled, actual)
					return captured
				}).Build()
				s := &Session{enableAutoUpdate: true, experimentalHealthValidation: enabled}
				response := &Response{}
				exitCode := 0
				s.processUpdate(t.Context(), Request{UpdateVersion: "v0.49.0"}, response, &exitCode)
				require.Equal(t, 1, calls)
				require.Equal(t, captured.Error(), response.Error)
			})
		})
	}
}
