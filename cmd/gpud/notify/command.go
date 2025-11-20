package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/urfave/cli"

	apiv1 "github.com/leptonai/gpud/api/v1"
	gpudcommon "github.com/leptonai/gpud/cmd/gpud/common"
	"github.com/leptonai/gpud/pkg/log"
	pkgmetadata "github.com/leptonai/gpud/pkg/metadata"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func CommandStartup(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting notify startup command")

	stateFile, err := gpudcommon.StateFileFromContext(cliContext)
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer rootCancel()
	machineID, err := pkgmetadata.ReadMachineID(rootCtx, dbRO)
	if err != nil {
		return err
	}

	endpoint, err := pkgmetadata.ReadMetadata(rootCtx, dbRO, pkgmetadata.MetadataKeyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to read endpoint: %w", err)
	}
	if endpoint == "" {
		log.Logger.Warn("endpoint is not set, skipping notification")
		os.Exit(0)
	}

	req := apiv1.NotificationRequest{
		ID:   machineID,
		Type: apiv1.NotificationTypeStartup,
	}

	dbToken, err := pkgmetadata.ReadToken(rootCtx, dbRO)
	if err != nil || dbToken == "" {
		log.Logger.Warn("machine not logged in, skipping notification")
		os.Exit(0)
	}

	return sendNotification(endpoint, req, dbToken)
}

func CommandShutdown(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	log.Logger.Debugw("starting notify shutdown command")

	stateFile, err := gpudcommon.StateFileFromContext(cliContext)
	if err != nil {
		return fmt.Errorf("failed to get state file: %w", err)
	}

	dbRW, err := sqlite.Open(stateFile)
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRW.Close()

	dbRO, err := sqlite.Open(stateFile, sqlite.WithReadOnly(true))
	if err != nil {
		return fmt.Errorf("failed to open state file: %w", err)
	}
	defer dbRO.Close()

	rootCtx, rootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer rootCancel()
	machineID, err := pkgmetadata.ReadMachineID(rootCtx, dbRO)
	if err != nil {
		return err
	}

	endpoint, err := pkgmetadata.ReadMetadata(rootCtx, dbRO, pkgmetadata.MetadataKeyEndpoint)
	if err != nil {
		return fmt.Errorf("failed to read endpoint: %w", err)
	}
	if endpoint == "" {
		log.Logger.Warn("endpoint is not set, skipping notification")
		os.Exit(0)
	}

	req := apiv1.NotificationRequest{
		ID:   machineID,
		Type: apiv1.NotificationTypeShutdown,
	}

	dbToken, err := pkgmetadata.ReadToken(rootCtx, dbRO)
	if err != nil || dbToken == "" {
		log.Logger.Warn("machine not logged in, skipping notification")
		os.Exit(0)
	}

	return sendNotification(endpoint, req, dbToken)
}

func sendNotification(endpoint string, req apiv1.NotificationRequest, token string) error {
	endpointURL := createNotificationURL(endpoint)
	u, err := url.Parse(endpointURL)
	if err != nil {
		return err
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("no host in endpoint URL: %s", endpoint)
	}

	rawPayload, _ := json.Marshal(&req)
	httpReq, err := http.NewRequest("POST", endpointURL, bytes.NewBuffer(rawPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Origin", host)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	response, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
		var errorResponse apiv1.NotificationResponse
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
		return fmt.Errorf("failed to send notification: %v", errorResponse)
	}
	return nil
}

// createNotificationURL creates a URL for the notification endpoint
func createNotificationURL(endpoint string) string {
	host := endpoint
	url, _ := url.Parse(endpoint)
	if url.Host != "" {
		host = url.Host
	}
	return fmt.Sprintf("https://%s/api/v1/notification", host)
}
