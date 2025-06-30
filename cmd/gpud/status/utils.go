package status

import (
	"fmt"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"

	cmdcommon "github.com/leptonai/gpud/cmd/common"
)

func checkLoginSuccess(loginSuccess, machineID string) error {
	if loginSuccess != "" {
		ts, err := strconv.ParseInt(loginSuccess, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse login success: %w", err)
		}
		loginTimeUTC := time.Unix(ts, 0)
		nowUTC := time.Now().UTC()
		loginTimeHumanized := humanize.RelTime(loginTimeUTC, nowUTC, "ago", "from now")
		fmt.Printf("%s login success at %s (machine id: %s)\n", cmdcommon.CheckMark, loginTimeHumanized, machineID)
	} else {
		fmt.Printf("%s login information not found\n", cmdcommon.CheckMark)
	}
	return nil
}
