package infiniband

import (
	"sync"

	nvidia_common "github.com/leptonai/gpud/components/accelerator/nvidia/common"
	"github.com/leptonai/gpud/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Config struct {
	PollInterval metav1.Duration `json:"poll_interval"`
	nvidia_common.ToolOverwrites
}

func (cfg *Config) Validate() error {
	return nil
}

var (
	defaultExpectedPortStatesMu sync.RWMutex
	defaultExpectedPortStates   = ExpectedPortStates{
		AtLeastPorts: 0,
		AtLeastRate:  0,
	}
)

func GetDefaultExpectedPortStates() ExpectedPortStates {
	defaultExpectedPortStatesMu.RLock()
	defer defaultExpectedPortStatesMu.RUnlock()
	return defaultExpectedPortStates
}

func SetDefaultExpectedPortStates(states ExpectedPortStates) {
	log.Logger.Infow("setting default expected port states", "at_least_ports", states.AtLeastPorts, "at_least_rate", states.AtLeastRate)

	defaultExpectedPortStatesMu.Lock()
	defer defaultExpectedPortStatesMu.Unlock()
	defaultExpectedPortStates = states
}

// Configures the expected state of the ports.
type ExpectedPortStates struct {
	// The minimum number of ports.
	// If not set, it defaults to the number of GPUs.
	AtLeastPorts int `json:"at_least_ports"`

	// The expected rate in Gb/sec.
	// If not set, it defaults to 200.
	AtLeastRate int `json:"at_least_rate"`
}
