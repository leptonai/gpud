package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/urfave/cli"
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
	rootCtx, rootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer rootCancel()
	endpoint := cliContext.GlobalString("endpoint")
	machineUID, err := GetUID(rootCtx)
	if err != nil {
		return err
	}
	req := payload{
		ID:   machineUID,
		Type: NotificationTypeStartup,
	}
	return notification(rootCtx, endpoint, req)
}

func cmdNotifyShutdown(cliContext *cli.Context) error {
	rootCtx, rootCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer rootCancel()
	endpoint := cliContext.GlobalString("endpoint")
	machineUID, err := GetUID(rootCtx)
	if err != nil {
		return err
	}
	req := payload{
		ID:   machineUID,
		Type: NotificationTypeShutdown,
	}
	return notification(rootCtx, endpoint, req)
}

func notification(ctx context.Context, endpoint string, req payload) error {
	type RespErr struct {
		Error  string `json:"error"`
		Status string `json:"status"`
	}
	rawPayload, _ := json.Marshal(&req)
	response, err := http.Post(fmt.Sprintf("https://%s/api/v1/notification", endpoint), "application/json", bytes.NewBuffer(rawPayload))
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
