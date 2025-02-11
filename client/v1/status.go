package v1

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/leptonai/gpud/manager/packages"
)

// GetPackageStatus fetches the GPUd package status from the GPUd admin API.
func GetPackageStatus(ctx context.Context, url string) ([]packages.PackageStatus, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %v received", resp.StatusCode)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var ret []packages.PackageStatus
	if err := json.Unmarshal(rawBody, &ret); err != nil {
		return nil, err
	}
	return ret, nil
}
