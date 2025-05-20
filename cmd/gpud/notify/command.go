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
	"github.com/leptonai/gpud/pkg/config"
	gpudstate "github.com/leptonai/gpud/pkg/gpud-state"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/sqlite"
)

func CommandStartup(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	stateFile, err := config.DefaultStateFile()
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
	machineID, err := gpudstate.ReadMachineIDWithFallback(rootCtx, dbRW, dbRO)
	if err != nil {
		return err
	}

	endpoint, err := gpudstate.ReadMetadata(rootCtx, dbRO, gpudstate.MetadataKeyEndpoint)
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

	return sendNotification(endpoint, req)
}

func CommandShutdown(cliContext *cli.Context) error {
	logLevel := cliContext.String("log-level")
	zapLvl, err := log.ParseLogLevel(logLevel)
	if err != nil {
		return err
	}
	log.Logger = log.CreateLogger(zapLvl, "")

	stateFile, err := config.DefaultStateFile()
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
	machineID, err := gpudstate.ReadMachineIDWithFallback(rootCtx, dbRW, dbRO)
	if err != nil {
		return err
	}

	endpoint, err := gpudstate.ReadMetadata(rootCtx, dbRO, gpudstate.MetadataKeyEndpoint)
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

	return sendNotification(endpoint, req)
}

func sendNotification(endpoint string, req apiv1.NotificationRequest) error {
	rawPayload, _ := json.Marshal(&req)
	response, err := http.Post(createNotificationURL(endpoint), "application/json", bytes.NewBuffer(rawPayload))
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
