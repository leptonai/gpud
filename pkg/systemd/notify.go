package systemd

import (
	"context"
	"os"

	sd "github.com/coreos/go-systemd/v22/daemon"

	"github.com/leptonai/gpud/pkg/log"
)

// savedNotifySocket stores the NOTIFY_SOCKET path so we can still send
// the stopping notification after the environment variable has been unset.
var savedNotifySocket string

// NotifyReady notifies systemd that the daemon is ready to serve requests.
// It also unsets NOTIFY_SOCKET from the process environment to prevent
// child processes from inheriting it and sending spurious notifications.
// ref. https://github.com/leptonai/gpud/issues/1215
func NotifyReady(_ context.Context) error {
	savedNotifySocket = os.Getenv("NOTIFY_SOCKET")

	// Pass unsetEnvironment=true so child processes won't inherit NOTIFY_SOCKET.
	notified, err := sd.SdNotify(true, sd.SdNotifyReady)
	log.Logger.Debugw("sd notification", "state", sd.SdNotifyReady, "notified", notified, "error", err)
	return err
}

// NotifyStopping notifies systemd that the daemon is about to be stopped.
func NotifyStopping(_ context.Context) error {
	// Temporarily restore NOTIFY_SOCKET for this call since it was
	// unset after NotifyReady to prevent child process inheritance.
	if savedNotifySocket != "" {
		os.Setenv("NOTIFY_SOCKET", savedNotifySocket)
		defer os.Unsetenv("NOTIFY_SOCKET")
	}
	return sdNotify(sd.SdNotifyStopping)
}

func sdNotify(state string) error {
	notified, err := sd.SdNotify(false, state)
	log.Logger.Debugw("sd notification", "state", state, "notified", notified, "error", err)
	return err
}
