package command

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	client "github.com/leptonai/gpud/client/v1"
	"github.com/leptonai/gpud/config"
	"github.com/leptonai/gpud/manager/packages"
	"github.com/leptonai/gpud/pkg/systemd"

	"github.com/urfave/cli"
)

func cmdStatus(cliContext *cli.Context) error {
	rootCtx, rootCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer rootCancel()

	if systemd.SystemctlExists() {
		active, err := systemd.IsActive("gpud.service")
		if err != nil {
			return err
		}
		if !active {
			fmt.Printf("%s gpud is not running\n", warningSign)
			return nil
		}
		fmt.Printf("%s gpud is running\n", checkMark)
	}
	fmt.Printf("%s successfully checked gpud status\n", checkMark)

	if err := client.BlockUntilServerReady(
		rootCtx,
		fmt.Sprintf("https://localhost:%d", config.DefaultGPUdPort),
	); err != nil {
		return err
	}
	fmt.Printf("%s successfully checked gpud health\n", checkMark)

	packageStatus, err := getStatus()
	if err != nil {
		fmt.Printf("%s failed to check package status: %v\n", warningSign, err)
		return err
	}
	for _, status := range packageStatus {
		statusSign := warningSign
		if status.Status {
			statusSign = checkMark
		}
		fmt.Printf("%s %v version: %v target version: %v, status: %v installed: %v\n", statusSign, status.Name, status.CurrentVersion, status.TargetVersion, status.Status, status.IsInstalled)
	}

	return nil
}

func getStatus() ([]packages.PackageStatus, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("https://localhost:%d/admin/packages", config.DefaultGPUdPort), nil)
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
