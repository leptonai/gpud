package pci

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/leptonai/gpud/components"
	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/pci/id"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/pci"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Output struct {
	// DevicesWithACS is the list of PCI devices with ACS enabled.
	DevicesWithACS pci.Devices `json:"devices_with_acs"`
}

func (o *Output) JSON() ([]byte, error) {
	return json.Marshal(o)
}

func ParseOutputJSON(data []byte) (*Output, error) {
	o := new(Output)
	if err := json.Unmarshal(data, o); err != nil {
		return nil, err
	}
	return o, nil
}

const (
	StateNameDevicesWithACS = "devices_with_acs"
	StateKeyData            = "data"
	StateKeyEncoding        = "encoding"
	StateValueEncodingJSON  = "json"
)

var currentVirtEnv host.VirtualizationEnvironment

func init() {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	currentVirtEnv, err = host.SystemdDetectVirt(ctx)
	cancel()
	if err != nil {
		log.Logger.Errorw("failed to detect virtualization environment", "error", err)
	}
}

func (o *Output) Events() ([]components.Event, error) {
	b, err := o.JSON()
	if err != nil {
		return nil, err
	}

	// in linux, and not in VM
	// then, check all ACS enabled devices
	//
	// Baremetal systems
	// IO virtualization (also known as VT-d or IOMMU) can interfere with GPU Direct by redirecting all
	// PCI point-to-point traffic to the CPU root complex, causing a significant performance reduction or even a hang.
	// If PCI switches have ACS enabled, it needs to be disabled.
	//
	// Virtual machines
	// Virtual machines require ACS to function, hence disabling ACS is not an option.
	//
	// ref. https://docs.nvidia.com/deeplearning/nccl/user-guide/docs/troubleshooting.html#pci-access-control-services-acs
	acsReasons := make([]string, 0)
	if currentVirtEnv.Type != "" &&
		!currentVirtEnv.IsKVM && // host is baremetal
		o.DevicesWithACS != nil &&
		len(o.DevicesWithACS) > 0 {
		for _, dev := range o.DevicesWithACS {
			// check whether ACS is enabled on PCI bridges
			if dev.AccessControlService == nil {
				continue
			}
			if dev.AccessControlService.ACSCtl.SrcValid {
				acsReasons = append(acsReasons, fmt.Sprintf("ACS is enabled on the PCI device %q (when host virt env type is %q)", dev.Name, currentVirtEnv.Type))
			}
		}
	}

	// no PCI device with Access Control Services (ACS) enabled or running in VM thus ACS is not an option
	if len(acsReasons) == 0 {
		return nil, nil
	}

	// polling happens periodically
	// so we truncate the timestamp to the nearest minute
	truncNowUTC := time.Now().UTC().Truncate(time.Minute)

	return []components.Event{
		{
			Time:    metav1.Time{Time: truncNowUTC},
			Name:    StateNameDevicesWithACS,
			Message: strings.Join(acsReasons, ", "),
			ExtraInfo: map[string]string{
				StateKeyData:     string(b),
				StateKeyEncoding: StateValueEncodingJSON,
			},
		},
	}, nil
}

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(id.Name, cfg.Query, Get)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func Get(ctx context.Context) (_ any, e error) {
	defer func() {
		if e != nil {
			components_metrics.SetGetFailed(id.Name)
		} else {
			components_metrics.SetGetSuccess(id.Name)
		}
	}()

	o := &Output{}

	devices, err := pci.List(ctx)
	if err != nil {
		return nil, err
	}
	if len(devices) > 0 {
		for _, dev := range devices {
			if dev.AccessControlService != nil {
				o.DevicesWithACS = append(o.DevicesWithACS, dev)
			}
		}
	}

	return o, nil
}
