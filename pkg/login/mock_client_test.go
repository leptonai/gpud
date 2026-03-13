package login

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/leptonai/gpud/api/v1"
)

func TestSendRequest_ExplicitEmptyNodeLabelsUsesRequestBody(t *testing.T) {
	mockey.PatchConvey("send request preserves explicit empty node labels", t, func() {
		mockey.Mock((*http.Client).Do).To(func(_ *http.Client, req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)

			assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
			assert.Contains(t, string(body), `"nodeLabels":{}`)

			respBody, err := json.Marshal(apiv1.LoginResponse{MachineID: "machine-123"})
			require.NoError(t, err)

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(respBody))),
				Header:     make(http.Header),
			}, nil
		}).Build()

		resp, err := sendRequest(context.Background(), "https://example.com/api/v1/login", apiv1.LoginRequest{
			Token:      "token",
			NodeLabels: map[string]string{},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "machine-123", resp.MachineID)
	})
}
