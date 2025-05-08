package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/pkg/server"
)

func GetMachineInfo(ctx context.Context, addr string, opts ...OpOption) (*apiv1.MachineInfo, error) {
	op := &Op{}
	if err := op.applyOpts(opts); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s%s", addr, server.URLPathMachineInfo), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	return getMachineInfo(createDefaultHTTPClient(), req)
}

func getMachineInfo(cli *http.Client, req *http.Request) (*apiv1.MachineInfo, error) {
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to %q: %w", req.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server not ready, response not 200")
	}

	var info apiv1.MachineInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode machine info: %w", err)
	}

	return &info, nil
}
