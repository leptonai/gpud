package pci

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/components/pci/id"
	"github.com/leptonai/gpud/pkg/common"
	"github.com/leptonai/gpud/pkg/eventstore"
	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/log"
	"github.com/leptonai/gpud/pkg/pci"
	"github.com/leptonai/gpud/pkg/query"
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

var (
	defaultPollerOnce sync.Once
	defaultPoller     query.Poller
)

// only set once since it relies on the kube client and specific port
func setDefaultPoller(cfg Config, eventBucket eventstore.Bucket) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			id.Name,
			cfg.Query,
			CreateGet(eventBucket),
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

var ErrNoEventStore = errors.New("no event store")

func CreateGet(eventBucket eventstore.Bucket) func(ctx context.Context) (_ any, e error) {
	return func(ctx context.Context) (_ any, e error) {
		if eventBucket == nil {
			return nil, ErrNoEventStore
		}

		// Virtual machines
		// Virtual machines require ACS to function, hence disabling ACS is not an option.
		//
		// ref. https://docs.nvidia.com/deeplearning/nccl/user-guide/docs/troubleshooting.html#pci-access-control-services-acs
		if currentVirtEnv.IsKVM {
			return nil, nil
		}
		// unknown virtualization environment
		if currentVirtEnv.Type == "" {
			return nil, nil
		}

		cctx, ccancel := context.WithTimeout(ctx, 10*time.Second)
		lastEvent, err := eventBucket.Latest(cctx)
		ccancel()
		if err != nil {
			return nil, err
		}

		nowUTC := time.Now().UTC()
		if lastEvent != nil && nowUTC.Sub(lastEvent.Time.Time) < 24*time.Hour {
			log.Logger.Debugw("found events thus skipping -- we only check once per day", "since", humanize.Time(nowUTC))
			return nil, nil
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
		devices, err := pci.List(ctx)
		if err != nil {
			return nil, err
		}

		ev := createEvent(nowUTC, devices)
		if ev == nil {
			return nil, nil
		}

		// no need to check duplicates
		// since we check once above

		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = eventBucket.Insert(cctx, *ev)
		cancel()
		if err != nil {
			return nil, err
		}

		return nil, nil
	}
}

func createEvent(time time.Time, devices []pci.Device) *components.Event {
	uuids := make([]string, 0)
	for _, dev := range devices {
		// check whether ACS is enabled on PCI bridges
		if dev.AccessControlService == nil {
			continue
		}
		if dev.AccessControlService.ACSCtl.SrcValid {
			uuids = append(uuids, dev.ID)
		}
	}

	if len(uuids) == 0 {
		return nil
	}

	return &components.Event{
		Time:    metav1.Time{Time: time.UTC()},
		Name:    "acs_enabled",
		Type:    common.EventTypeWarning,
		Message: fmt.Sprintf("host virt env is %q, ACS is enabled on the following PCI devices: %s", currentVirtEnv.Type, strings.Join(uuids, ", ")),
	}
}
