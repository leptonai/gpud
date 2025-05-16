package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/urfave/cli"

	"github.com/leptonai/gpud/pkg/config"
	gpudstate "github.com/leptonai/gpud/pkg/gpud-state"
	"github.com/leptonai/gpud/pkg/httputil"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/sqlite"
)

type NotificationType string

const (
	NotificationTypeShutdown NotificationType = "shutdown"
	NotificationTypeStartup  NotificationType = "startup"
)

type payload struct {
	ID   string           `json:"id"`
	Type NotificationType `json:"type"`
}

func cmdNotifyStartup(cliContext *cli.Context) error {
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

	req := payload{
		ID:   machineID,
		Type: NotificationTypeStartup,
	}

	return notification(endpoint, req)
}

func cmdNotifyShutdown(cliContext *cli.Context) error {
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

	req := payload{
		ID:   machineID,
		Type: NotificationTypeShutdown,
	}

	return notification(endpoint, req)
}

func notification(endpoint string, req payload) error {
	type RespErr struct {
		Error  string `json:"error"`
		Status string `json:"status"`
	}
	rawPayload, _ := json.Marshal(&req)

	u, err := httputil.CreateURL("https", endpoint, "/api/v1/notification")
	if err != nil {
		return fmt.Errorf("failed to create notification URL: %w", err)
	}

	response, err := http.Post(u, "application/json", bytes.NewBuffer(rawPayload))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("error reading response body: %w", err)
		}
		var errorResponse RespErr
		err = json.Unmarshal(body, &errorResponse)
		if err != nil {
			return fmt.Errorf("Error parsing error response: %v\nResponse body: %s", err, body)
		}
		return fmt.Errorf("failed to send notification: %v", errorResponse)
	}
	return nil
}
