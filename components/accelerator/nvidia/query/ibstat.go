package query

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func IbstatExists() bool {
	p, err := exec.LookPath("ibstat")
	if err != nil {
		return false
	}
	return p != ""
}

// Checks if "/sys/class/infiniband" directory exists.
func InfinibandClassExists() bool {
	info, err := os.Stat("/sys/class/infiniband")
	return err == nil && info.IsDir()
}

func RunIbstat(ctx context.Context) (*IbstatOutput, error) {
	p, err := exec.LookPath("ibstat")
	if err != nil {
		return nil, fmt.Errorf("ibstat not found (%w)", err)
	}
	b, err := exec.CommandContext(ctx, p).CombinedOutput()
	if err != nil {
		return nil, err
	}
	o := &IbstatOutput{
		Raw: string(b),
	}
	if err := ValidateIbstatOutput(o.Raw); err != nil {
		o.Errors = append(o.Errors, err.Error())
	}
	return o, nil
}

var (
	ErrIbstatOutputBrokenStateDown        = errors.New("ibstat output unexpected; found State: Down")
	ErrIbstatOutputBrokenPhysicalDisabled = errors.New("ibstat output seems broken; found Physical state: Disabled")
)

func ValidateIbstatOutput(s string) error {
	if strings.Contains(s, "State: Down") {
		return ErrIbstatOutputBrokenStateDown
	}

	// needs
	// "ip link set <dev> up"
	if strings.Contains(s, "Physical state: Disabled") {
		return ErrIbstatOutputBrokenPhysicalDisabled
	}

	return nil
}

type IbstatOutput struct {
	Raw    string   `json:"raw"`
	Errors []string `json:"errors,omitempty"`
}
