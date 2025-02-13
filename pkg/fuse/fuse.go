// Package fuse provides a client for the FUSE (Filesystem in Userspace) protocol.
package fuse

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/leptonai/gpud/pkg/disk"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/olekukonko/tablewriter"
)

const DefaultConnectionsDir = "/sys/fs/fuse/connections"

// Represents the information about a FUSE connection.
// ref. https://www.kernel.org/doc/Documentation/filesystems/fuse.txt
type ConnectionInfo struct {
	// The device number of the connection.
	// The process and mount information can be found in the corresponding "mountinfo" file
	// at "/proc/[process id]/mountinfo".
	// e.g.,
	// "1573 899 0:53 / /mnt/remote-volume/dev rw,nosuid,nodev,relatime shared:697 - fuse.testfs TestFS:test"
	Device int `json:"device"`

	// Fstype is the filesystem type of the connection.
	// Derived from "/proc/self/mountinfo".
	Fstype string `json:"fstype"`

	// DeviceName is the device name of the connection.
	// Derived from "/proc/self/mountinfo".
	DeviceName string `json:"device_name"`

	CongestionThreshold int `json:"congestion_threshold"`
	// CongestedPercent is the percentage of the congestion threshold that is congested
	// based on the waiting value.
	CongestedPercent float64 `json:"congested_percent"`

	MaxBackground int `json:"max_background"`
	// MaxBackgroundPercent is the percentage of the max background that is used
	// based on the waiting value.
	MaxBackgroundPercent float64 `json:"max_background_percent"`

	// The number of requests which are waiting to be transferred to
	// userspace or being processed by the filesystem daemon.  If there is
	// no filesystem activity and 'waiting' is non-zero, then the
	// filesystem is hung or deadlocked.
	Waiting int `json:"waiting"`
}

func (info ConnectionInfo) JSON() ([]byte, error) {
	return json.Marshal(info)
}

type ConnectionInfos []ConnectionInfo

func (infos ConnectionInfos) RenderTable(wr io.Writer) {
	table := tablewriter.NewWriter(wr)
	table.SetHeader([]string{"Device", "Fstype", "Device Name", "Congestion Threshold", "Congested %", "Max Background Threshold", "Max Background %", "Waiting"})

	for _, info := range infos {
		table.Append([]string{
			fmt.Sprintf("%d", info.Device),
			info.Fstype,
			info.DeviceName,

			fmt.Sprintf("%d", info.CongestionThreshold),
			fmt.Sprintf("%.2f%%", info.CongestedPercent),

			fmt.Sprintf("%d", info.MaxBackground),
			fmt.Sprintf("%.2f%%", info.MaxBackgroundPercent),

			fmt.Sprintf("%d", info.Waiting),
		})
	}

	table.Render()
}

// ListConnections retrieves the connection information for all FUSE connections.
func ListConnections() (ConnectionInfos, error) {
	infos, err := listConnections(DefaultConnectionsDir)
	if err != nil {
		return nil, err
	}

	for i, info := range infos {
		fsType, dev, err := disk.FindFsTypeAndDeviceByMinorNumber(info.Device)
		if err != nil {
			log.Logger.Warnw("failed to find fs type and device by minor number", "error", err)
			continue
		}
		infos[i].Fstype = fsType
		infos[i].DeviceName = dev
	}

	return infos, nil
}

func listConnections(connectionsDir string) ([]ConnectionInfo, error) {
	// read all the sub-directories in the connectionsDir
	entries, err := os.ReadDir(connectionsDir)
	if err != nil {
		return nil, err
	}

	infos := make([]ConnectionInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(connectionsDir, entry.Name())

		device, err := strconv.Atoi(strings.TrimSpace(entry.Name()))
		if err != nil {
			return nil, err
		}

		congestionThresholdRaw, err := os.ReadFile(filepath.Join(dir, "congestion_threshold"))
		if err != nil {
			return nil, err
		}
		congestionThreshold, err := strconv.Atoi(strings.TrimSpace(string(congestionThresholdRaw)))
		if err != nil {
			return nil, err
		}

		maxBackgroundRaw, err := os.ReadFile(filepath.Join(dir, "max_background"))
		if err != nil {
			return nil, err
		}
		maxBackground, err := strconv.Atoi(strings.TrimSpace(string(maxBackgroundRaw)))
		if err != nil {
			return nil, err
		}

		waitingRaw, err := os.ReadFile(filepath.Join(dir, "waiting"))
		if err != nil {
			return nil, err
		}
		waiting, err := strconv.Atoi(strings.TrimSpace(string(waitingRaw)))
		if err != nil {
			return nil, err
		}

		congestedPercent := float64(waiting) / float64(congestionThreshold) * 100
		maxBackgroundPercent := float64(waiting) / float64(maxBackground) * 100

		info := ConnectionInfo{
			Device:               device,
			CongestionThreshold:  congestionThreshold,
			CongestedPercent:     congestedPercent,
			MaxBackground:        maxBackground,
			MaxBackgroundPercent: maxBackgroundPercent,
			Waiting:              waiting,
		}
		infos = append(infos, info)
	}
	return infos, nil
}
