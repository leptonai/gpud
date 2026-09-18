package systemd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServiceCommandPrefix(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "systemctl")
	// Both operations must address the selected unit through the supplied prefix.
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
case "$1" in
is-active)
  [ "$2" = rke2-agent ] || exit 1
  printf 'active\n'
  ;;
show)
  [ "$2" = --property=InactiveExitTimestamp ] && [ "$3" = rke2-agent ] || exit 1
  printf 'InactiveExitTimestamp=Mon 2020-01-06 15:04:05 UTC\n'
  ;;
*) exit 1 ;;
esac
`), 0700))
	prefix := "sh '" + script + "'"
	active, err := IsActiveWithCommand("rke2-agent", prefix)
	require.NoError(t, err)
	require.True(t, active)
	uptime, err := GetUptimeWithCommand("rke2-agent", prefix)
	require.NoError(t, err)
	require.NotNil(t, uptime)
	require.Greater(t, *uptime, time.Hour)
	_, err = IsActiveWithCommand("containerd", prefix)
	require.Error(t, err)
	_, err = GetUptimeWithCommand("containerd", prefix)
	require.Error(t, err)
}
