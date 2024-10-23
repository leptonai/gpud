// Package host provides the host information.
package host

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
)

// Fetches the UUIF of the machine host, using the "dmidecode".
// ref.
// UUID=$(dmidecode -t 1 | grep -i UUID | awk '{print $2}')
func UUID(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "dmidecode", "-t", "system")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 6 && line[:6] == "UUID: " {
			uid := line[6:]
			if strings.Contains(uid, "Not Settable") {
				generateUUID, _ := uuid.NewUUID()
				uid = generateUUID.String()
			}
			return uid, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("UUID not found")
}
