package common

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

// ConsumeEnrollmentToken reads the credential once and removes it from the
// environment before login or startup can launch child processes.
func ConsumeEnrollmentToken(c *cli.Context) (string, error) {
	token := c.String("token")
	// An empty Secret means no login, while an explicit --token= must still
	// report an invalid credential. cli.StringFlag.EnvVar conflates the two.
	if !c.IsSet("token") && token == "" {
		token = os.Getenv("GPUD_TOKEN")
	}
	// Clear even an overridden token: plugins and host commands inherit the
	// process environment and can publish their output to the control plane.
	if err := os.Unsetenv("GPUD_TOKEN"); err != nil {
		return "", fmt.Errorf("failed to clear enrollment token environment: %w", err)
	}
	return token, nil
}
