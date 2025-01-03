package pci

import (
	"context"
	"fmt"
	"sync"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	"github.com/leptonai/gpud/components/pci/id"
	"github.com/leptonai/gpud/components/pci/state"
	"github.com/leptonai/gpud/components/query"
	"github.com/leptonai/gpud/log"
	"github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/pci"

	"github.com/dustin/go-humanize"
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
func setDefaultPoller(cfg Config) {
	defaultPollerOnce.Do(func() {
		defaultPoller = query.New(
			id.Name,
			cfg.Query,
			CreateGet(cfg),
			nil,
		)
	})
}

func getDefaultPoller() query.Poller {
	return defaultPoller
}

func CreateGet(cfg Config) func(ctx context.Context) (_ any, e error) {
	return func(ctx context.Context) (_ any, e error) {
		defer func() {
			if e != nil {
				components_metrics.SetGetFailed(id.Name)
			} else {
				components_metrics.SetGetSuccess(id.Name)
			}
		}()

		devices, err := pci.List(ctx)
		if err != nil {
			return nil, err
		}

		nowUTC := time.Now().UTC()
		since := nowUTC.Add(-24 * time.Hour)

		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		evs, err := state.ReadEvents(
			cctx,
			cfg.Query.State.DBRO,
			state.WithSince(since),
		)
		cancel()
		if err != nil {
			return nil, err
		}
		if len(evs) > 0 {
			log.Logger.Debugw("found events thus skipping", "since", humanize.Time(since))
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
		uuids := make([]string, 0)
		if currentVirtEnv.Type != "" && !currentVirtEnv.IsKVM {
			for _, dev := range devices {
				// check whether ACS is enabled on PCI bridges
				if dev.AccessControlService == nil {
					continue
				}
				if dev.AccessControlService.ACSCtl.SrcValid {
					uuids = append(uuids, dev.ID)
				}
			}
		}

		if len(uuids) == 0 {
			return nil, nil
		}

		acsReasons := append([]string{fmt.Sprintf("host virt env is %q, ACS is enabled on the following PCI devices", currentVirtEnv.Type)}, uuids...)
		cctx, cancel = context.WithTimeout(ctx, 10*time.Second)
		err = state.InsertEvent(cctx, cfg.Query.State.DBRW, state.Event{
			UnixSeconds: nowUTC.Unix(),
			DataSource:  id.Name,
			EventType:   "acs_enabled",
			Reasons:     acsReasons,
		})
		cancel()
		if err != nil {
			return nil, err
		}

		return nil, nil
	}
}
