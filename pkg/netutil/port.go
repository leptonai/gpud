package netutil

import (
	"fmt"
	"net"
	"time"
)

// IsPortOpen checks if the TCP port is open/used.
// It returns true if the port is open/used, otherwise false.
func IsPortOpen(port int) bool {
	// check if the TCP port is open/used
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 3*time.Second)
	if err != nil {
		return false
	}
	defer func() {
		_ = conn.Close()
	}()

	return true
}
