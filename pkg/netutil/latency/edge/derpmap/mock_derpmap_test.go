package derpmap

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tailscale.com/tailcfg"
)

type errReadCloser struct {
	err error
}

func (e *errReadCloser) Read(_ []byte) (int, error) { return 0, e.err }
func (e *errReadCloser) Close() error               { return nil }

func TestDownloadTailcaleDERPMap_SuccessWithMockey(t *testing.T) {
	mockey.PatchConvey("downloads and parses derp map", t, func() {
		expected := tailcfg.DERPMap{
			Regions: map[int]*tailcfg.DERPRegion{
				1: {
					RegionID:   1,
					RegionCode: "sea",
					RegionName: "Seattle",
					Nodes: []*tailcfg.DERPNode{
						{
							Name:     "1a",
							RegionID: 1,
							HostName: "derp.example.com",
							IPv4:     "1.1.1.1",
							IPv6:     "2001:db8::1",
							STUNPort: 3478,
							DERPPort: 443,
						},
					},
				},
			},
		}
		raw, err := json.Marshal(expected)
		require.NoError(t, err)

		mockey.Mock(http.Get).To(func(_ string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(raw)),
			}, nil
		}).Build()

		got, err := DownloadTailcaleDERPMap()
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Contains(t, got.Regions, 1)
		assert.Equal(t, "Seattle", got.Regions[1].RegionName)
		assert.Equal(t, "sea", got.Regions[1].RegionCode)
	})
}

func TestDownloadTailcaleDERPMap_GetErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("propagates http get error", t, func() {
		getErr := errors.New("dial failed")
		mockey.Mock(http.Get).To(func(_ string) (*http.Response, error) {
			return nil, getErr
		}).Build()

		got, err := DownloadTailcaleDERPMap()
		require.Error(t, err)
		require.ErrorIs(t, err, getErr)
		assert.Nil(t, got)
	})
}

func TestDownloadTailcaleDERPMap_ReadBodyErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("returns read body error", t, func() {
		readErr := errors.New("read failed")
		mockey.Mock(http.Get).To(func(_ string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       &errReadCloser{err: readErr},
			}, nil
		}).Build()

		got, err := DownloadTailcaleDERPMap()
		require.Error(t, err)
		require.ErrorIs(t, err, readErr)
		assert.Nil(t, got)
	})
}

func TestDownloadTailcaleDERPMap_UnmarshalErrorWithMockey(t *testing.T) {
	mockey.PatchConvey("returns unmarshal error", t, func() {
		mockey.Mock(http.Get).To(func(_ string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("{invalid-json")),
			}, nil
		}).Build()

		got, err := DownloadTailcaleDERPMap()
		require.Error(t, err)
		assert.Nil(t, got)
	})
}
