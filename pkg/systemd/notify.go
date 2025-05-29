package systemd

import (
	"context"

	sd "github.com/coreos/go-systemd/v22/daemon"
	"github.com/leptonai/gpud/pkg/log"
)

// NotifyReady notifies systemd that the daemon is ready to serve requests
func NotifyReady(_ context.Context) error {
	return sdNotify(sd.SdNotifyReady)
}

// NotifyStopping notifies systemd that the daemon is about to be stopped
func NotifyStopping(_ context.Context) error {
	return sdNotify(sd.SdNotifyStopping)
}

func sdNotify(state string) error {
	notified, err := sd.SdNotify(false, state)
	log.Logger.Debugw("sd notification", "state", state, "notified", notified, "error", err)
	return err
}
