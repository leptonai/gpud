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

	for {
		packageStatus, err := getStatus()
		if err != nil {
			fmt.Printf("%s failed to get package status: %v\n", warningSign, err)
			return err
		}
		if len(packageStatus) == 0 {
			fmt.Printf("no packages found\n")
			return nil
		}
		if statusWatch {
			fmt.Print("\033[2J\033[H")
		}
		var totalTime int64
		var progress int64
		for _, status := range packageStatus {
			totalTime += status.TotalTime.Milliseconds()
			progress += status.TotalTime.Milliseconds() * int64(status.Progress) / 100
		}

		var totalProgress int64
		if totalTime != 0 {
			totalProgress = progress * 100 / totalTime
		}
		fmt.Printf("Total progress: %v%%, Estimate time left: %v\n", totalProgress, time.Duration(totalTime-progress)*time.Millisecond)
		if !statusWatch {
			break
		}
		time.Sleep(3 * time.Second)
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
