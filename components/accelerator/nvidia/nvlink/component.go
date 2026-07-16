// Package nvlink monitors the NVIDIA per-GPU nvlink devices.
package nvlink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/leptonai/gpud/api/v1"
	"github.com/leptonai/gpud/components"
	"github.com/leptonai/gpud/pkg/eventstore"
	pkghost "github.com/leptonai/gpud/pkg/host"
	"github.com/leptonai/gpud/pkg/kmsg"
	"github.com/leptonai/gpud/pkg/log"
	nvmlerrors "github.com/leptonai/gpud/pkg/nvidia/errors"
	nvidianvml "github.com/leptonai/gpud/pkg/nvidia/nvml"
	"github.com/leptonai/gpud/pkg/nvidia/nvml/device"
)

// Name is the name of the NVIDIA NVLink component.
const Name = "accelerator-nvidia-nvlink"

const (
	defaultCheckInterval        = time.Minute
	defaultCheckStaleAfter      = 2 * defaultCheckInterval
	defaultStateUpdateInterval  = 30 * time.Second
	defaultKmsgEventDedupWindow = 5 * time.Minute
)

var _ components.Component = &component{}

type component struct {
	ctx    context.Context
	cancel context.CancelFunc

	getTimeNowFunc  func() time.Time
	getBootTimeFunc func() time.Time

	nvmlInstance             nvidianvml.Instance
	getNVLinkFunc            func(uuid string, dev device.Device) (NVLink, error)
	getPeerNVLinkP2PStatusFn func(dev device.Device, peer device.Device) (string, error)
	getThresholdsFunc        func() ExpectedLinkStates
	eventBucket              eventstore.Bucket
	kmsgSyncer               *kmsg.Syncer
	readAllKmsg              func(context.Context) ([]kmsg.Message, error)

	lastMu               sync.RWMutex
	lastCheckResult      *checkResult
	checksInFlight       map[*checkResult]time.Time
	monitoringStartedAt  time.Time
	lastCheckCompletedAt time.Time

	mu        sync.RWMutex
	currState apiv1.HealthState
}

// New creates a NVIDIA NVLink component.
func New(gpudInstance *components.GPUdInstance) (components.Component, error) {
	cctx, ccancel := context.WithCancel(gpudInstance.RootCtx)
	c := &component{
		ctx:    cctx,
		cancel: ccancel,
		getTimeNowFunc: func() time.Time {
			return time.Now().UTC()
		},
		getBootTimeFunc:          pkghost.BootTime,
		nvmlInstance:             gpudInstance.NVMLInstance,
		getNVLinkFunc:            GetNVLink,
		getPeerNVLinkP2PStatusFn: getPeerNVLinkP2PStatus,
		getThresholdsFunc:        GetDefaultExpectedLinkStates,
	}

	if gpudInstance.EventStore != nil {
		var err error
		c.eventBucket, err = gpudInstance.EventStore.Bucket(Name, eventstore.WithDisablePurge())
		if err != nil {
			ccancel()
			return nil, err
		}

		if os.Geteuid() == 0 {
			bootID := pkghost.BootID()
			if bootID == "" {
				bootTime := c.getBootTimeFunc()
				if !bootTime.IsZero() {
					bootID = bootTime.Format(time.RFC3339Nano)
				} else {
					bootID = c.getTimeNowFunc().Format(time.RFC3339Nano)
				}
			}
			c.kmsgSyncer, err = kmsg.NewSyncer(
				cctx,
				func(line string) (string, string) { return matchWithBootID(line, bootID) },
				c.eventBucket,
				kmsg.WithCacheKeyTruncateSeconds(int(defaultKmsgEventDedupWindow.Seconds())),
			)
			if err != nil {
				c.eventBucket.Close()
				ccancel()
				return nil, err
			}
		}
	}
	if gpudInstance.EventStore == nil && runtime.GOOS == "linux" && os.Geteuid() == 0 {
		c.readAllKmsg = kmsg.ReadAll
	}
	return c, nil
}

func (c *component) Name() string { return Name }

func (c *component) Tags() []string {
	return []string{
		"accelerator",
		"gpu",
		"nvidia",
		Name,
	}
}

func (c *component) IsSupported() bool {
	if c.nvmlInstance == nil {
		return false
	}
	return c.nvmlInstance.NVMLExists() && c.nvmlInstance.ProductName() != ""
}

func (c *component) Start() error {
	if c.eventBucket != nil {
		if err := c.updateCurrentState(); err != nil {
			log.Logger.Errorw("failed to fetch current events", "error", err)
		}
	}
	c.markMonitoringStarted()

	go func() {
		ticker := time.NewTicker(defaultCheckInterval)
		defer ticker.Stop()

		for {
			_ = c.Check()

			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	if c.eventBucket != nil {
		go func() {
			ticker := time.NewTicker(defaultStateUpdateInterval)
			defer ticker.Stop()

			for {
				select {
				case <-c.ctx.Done():
					return
				case <-ticker.C:
				}

				if err := c.updateCurrentState(); err != nil {
					log.Logger.Errorw("failed to fetch current events", "error", err)
				}
			}
		}()
	}
	return nil
}

func (c *component) LastHealthStates() apiv1.HealthStates {
	c.mu.RLock()
	currState := c.currState
	c.mu.RUnlock()
	if currState.Health != "" {
		return apiv1.HealthStates{currState}
	}

	if state := c.watchdogHealthState(); state != nil {
		return apiv1.HealthStates{*state}
	}

	c.lastMu.RLock()
	lastCheckResult := c.lastCheckResult
	c.lastMu.RUnlock()
	return lastCheckResult.HealthStates()
}

func (c *component) Events(ctx context.Context, since time.Time) (apiv1.Events, error) {
	if c.eventBucket == nil {
		return nil, nil
	}
	events, err := c.eventBucket.Get(ctx, since)
	if err != nil {
		return nil, err
	}
	return events.Events(), nil
}

func (c *component) Close() error {
	log.Logger.Debugw("closing component")

	c.cancel()

	if c.kmsgSyncer != nil {
		c.kmsgSyncer.Close()
	}
	if c.eventBucket != nil {
		c.eventBucket.Close()
	}

	return nil
}

func (c *component) Check() components.CheckResult {
	log.Logger.Infow("checking nvidia gpu nvlink")

	startedAt := c.getTimeNowFunc()
	cr := &checkResult{ts: startedAt}
	c.beginCheck(cr)
	if c.getThresholdsFunc != nil {
		thresholds := c.getThresholdsFunc()
		cr.ExpectedLinkStates = &thresholds
	}
	defer c.finishCheck(cr)

	if c.nvmlInstance == nil {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML instance is nil"
		return cr
	}
	if !c.nvmlInstance.NVMLExists() {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML library is not loaded"
		return cr
	}
	// Check for NVML initialization errors first.
	// This handles cases like "error getting device handle for index 'N': Unknown Error"
	// which corresponds to nvidia-smi showing "Unable to determine the device handle for GPU".
	if err := c.nvmlInstance.InitError(); err != nil {
		cr.health = apiv1.HealthStateTypeUnhealthy
		cr.reason = fmt.Sprintf("NVML initialization error: %v", err)
		cr.suggestedActions = &apiv1.SuggestedActions{
			RepairActions: []apiv1.RepairActionType{
				apiv1.RepairActionTypeRebootSystem,
			},
		}
		return cr
	}
	if c.nvmlInstance.ProductName() == "" {
		cr.health = apiv1.HealthStateTypeHealthy
		cr.reason = "NVIDIA NVML is loaded but GPU is not detected (missing product name)"
		return cr
	}
	// gpud scan calls Check directly without Start or an event store. New only
	// installs this reader in that mode, leaving daemon checks on the syncer path.
	if c.readAllKmsg != nil {
		cctx, ccancel := context.WithTimeout(c.ctx, 30*time.Second)
		kmsgs, err := c.readAllKmsg(cctx)
		ccancel()
		if err != nil {
			cr.err = err
			cr.reason = "failed to read kmsg"
			cr.health = apiv1.HealthStateTypeUnhealthy
			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}
		cr.KmsgScanned = true
		type kmsgEventWindow struct {
			name, message string
			window        int64
		}
		seen := make(map[kmsgEventWindow]struct{})
		for _, message := range kmsgs {
			eventName, eventMessage := Match(message.Message)
			if eventName == "" {
				continue
			}
			key := kmsgEventWindow{
				name:    eventName,
				message: eventMessage,
				window:  message.Timestamp.Unix() / int64(defaultKmsgEventDedupWindow/time.Second),
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			cr.MatchedKmsgs = append(cr.MatchedKmsgs, message)
		}
		if len(cr.MatchedKmsgs) > 0 {
			state := unhealthyRebootState(cr.ts, postRxDetectFailureMessage)
			cr.health = state.Health
			cr.reason = state.Reason
			cr.suggestedActions = state.SuggestedActions
			return cr
		}
	}

	devs := c.nvmlInstance.Devices()
	// Only expect NVLink by default on multi-GPU hosts that advertise NVIDIA
	// fabric support. This keeps 1-GPU machines safe: for a single GPU,
	// `nvidia-smi topo -p2p n` legitimately shows only the self entry:
	//
	//   $ nvidia-smi topo -p2p n
	//        GPU0
	//   GPU0 X
	//
	// On a healthy 2-GPU NVLink-capable host, peer entries should be `OK`:
	//
	//   $ nvidia-smi topo -p2p n
	//        GPU0 GPU1
	//   GPU0 X    OK
	//   GPU1 OK   X
	//
	// This flag is intentionally conservative: it only enables the fallback for
	// the obvious "zero GPUs have active NVLink" case. If some GPUs are active
	// and some are not, operators must still configure ExpectedLinkStates to make
	// partial degradation fail health checks.
	cr.SystemExpectedNVLink = len(devs) > 1 && (c.nvmlInstance.FabricManagerSupported() || c.nvmlInstance.FabricStateSupported())
	sortedUUIDs := make([]string, 0, len(devs))
	for uuid := range devs {
		sortedUUIDs = append(sortedUUIDs, uuid)
	}
	sort.Strings(sortedUUIDs)

	if c.getPeerNVLinkP2PStatusFn != nil && len(sortedUUIDs) > 1 {
		peerNVLinkOKGPUUUIDs := make(map[string]struct{})
		peerNVLinkObservedStatusCodes := make(map[string]struct{})
		cr.PeerNVLinkExpectedPairCount = len(sortedUUIDs) * (len(sortedUUIDs) - 1) / 2

		// WHY: per-GPU NVLink port state can stay FEATURE_ENABLED even when the
		// fabric is unusable between GPU peers. In the field this showed up as
		// `nvidia-smi topo -p2p n` reporting only `NS` peer entries while GPUD
		// still rendered every GPU as NVLink enabled/supported. Probe pairwise
		// NVLink P2P status here so the component can catch topology-level
		// failures that per-port state alone misses.
		for i := 0; i < len(sortedUUIDs); i++ {
			for j := i + 1; j < len(sortedUUIDs); j++ {
				uuid := sortedUUIDs[i]
				peerUUID := sortedUUIDs[j]

				statusCode, err := c.getPeerNVLinkP2PStatusFn(devs[uuid], devs[peerUUID])
				if err != nil {
					log.Logger.Debugw(
						"failed to get nvlink peer-to-peer status",
						"uuid", uuid,
						"peer_uuid", peerUUID,
						"error", err,
					)
					continue
				}

				cr.PeerNVLinkProbePairCount++
				peerNVLinkObservedStatusCodes[statusCode] = struct{}{}
				if statusCode == p2pStatusOK {
					cr.PeerNVLinkOKPairCount++
					peerNVLinkOKGPUUUIDs[uuid] = struct{}{}
					peerNVLinkOKGPUUUIDs[peerUUID] = struct{}{}
				}
			}
		}

		if len(peerNVLinkOKGPUUUIDs) > 0 {
			cr.PeerNVLinkOKGPUUUIDs = sortedKeys(peerNVLinkOKGPUUUIDs)
		}
		if len(peerNVLinkObservedStatusCodes) > 0 {
			cr.PeerNVLinkObservedStatusCodes = sortedKeys(peerNVLinkObservedStatusCodes)
		}
	}

	for _, uuid := range sortedUUIDs {
		dev := devs[uuid]
		nvLink, err := c.getNVLinkFunc(uuid, dev)
		if err != nil {
			cr.err = err
			cr.health = apiv1.HealthStateTypeUnhealthy
			cr.reason = "error getting nvlink"

			if errors.Is(err, nvmlerrors.ErrGPURequiresReset) {
				cr.reason = nvmlerrors.ErrGPURequiresReset.Error()
				cr.suggestedActions = &apiv1.SuggestedActions{
					Description: nvmlerrors.ErrGPURequiresReset.Error(),
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeRebootSystem,
					},
				}
			}

			if errors.Is(err, nvmlerrors.ErrGPULost) {
				cr.reason = nvmlerrors.ErrGPULost.Error()
				cr.suggestedActions = &apiv1.SuggestedActions{
					Description: nvmlerrors.ErrGPULost.Error(),
					RepairActions: []apiv1.RepairActionType{
						apiv1.RepairActionTypeRebootSystem,
					},
				}
			}

			log.Logger.Warnw(cr.reason, "error", cr.err)
			return cr
		}

		cr.NVLinks = append(cr.NVLinks, nvLink)

		labels := prometheus.Labels{"uuid": uuid}
		if nvLink.Supported {
			metricSupported.With(labels).Set(1.0)
		} else {
			metricSupported.With(labels).Set(0.0)
			metricFeatureEnabled.With(labels).Set(0.0)
			metricReplayErrors.With(labels).Set(0.0)
			metricRecoveryErrors.With(labels).Set(0.0)
			metricCRCErrors.With(labels).Set(0.0)
			cr.UnsupportedNVLinkUUIDs = append(cr.UnsupportedNVLinkUUIDs, uuid)
			continue
		}

		featureEnabled := len(nvLink.States) > 0 && nvLink.States.AllFeatureEnabled()
		if featureEnabled {
			metricFeatureEnabled.With(labels).Set(1.0)
			cr.ActiveNVLinkUUIDs = append(cr.ActiveNVLinkUUIDs, uuid)
		} else {
			metricFeatureEnabled.With(labels).Set(0.0)
			cr.InactiveNVLinkUUIDs = append(cr.InactiveNVLinkUUIDs, uuid)
		}
		metricReplayErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalReplayErrors()))
		metricRecoveryErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalRecoveryErrors()))
		metricCRCErrors.With(prometheus.Labels{"uuid": uuid}).Set(float64(nvLink.States.TotalCRCErrors()))
	}

	if len(cr.ActiveNVLinkUUIDs) > 0 {
		sort.Strings(cr.ActiveNVLinkUUIDs)
	}
	if len(cr.InactiveNVLinkUUIDs) > 0 {
		sort.Strings(cr.InactiveNVLinkUUIDs)
	}
	if len(cr.UnsupportedNVLinkUUIDs) > 0 {
		sort.Strings(cr.UnsupportedNVLinkUUIDs)
	}

	cr.health = apiv1.HealthStateTypeHealthy
	cr.reason = fmt.Sprintf("all %d GPU(s) were checked, no nvlink issue found", len(devs))

	evaluateHealthStateWithThresholds(cr)

	return cr
}

var _ components.CheckResult = &checkResult{}

type checkResult struct {
	KmsgScanned  bool           `json:"-"`
	MatchedKmsgs []kmsg.Message `json:"-"`

	// NVLinks contains detailed NVLink information for all GPUs checked
	NVLinks []NVLink `json:"nvlinks,omitempty"`

	// ActiveNVLinkUUIDs lists GPUs where NVLink is supported AND all links have FeatureEnabled=true
	// (i.e., len(States) > 0 && States.AllFeatureEnabled() == true)
	// These GPUs have fully operational NVLink connectivity
	ActiveNVLinkUUIDs []string `json:"active_nvlink_uuids,omitempty"`

	// InactiveNVLinkUUIDs lists GPUs where NVLink is supported BUT at least one link has FeatureEnabled=false
	// This corresponds to nvidia-smi showing "all links are inActive"
	// Common causes: disabled via driver, fabric manager issues, NVSwitch connectivity problems
	InactiveNVLinkUUIDs []string `json:"inactive_nvlink_uuids,omitempty"`

	// UnsupportedNVLinkUUIDs lists GPUs that do not support NVLink at hardware/firmware level
	UnsupportedNVLinkUUIDs []string `json:"unsupported_nvlink_uuids,omitempty"`

	// ExpectedLinkStates defines the threshold for how many GPUs must have active NVLink
	// Used by evaluateHealthStateWithThresholds to determine if the system is healthy
	ExpectedLinkStates *ExpectedLinkStates `json:"expected_link_states,omitempty"`

	// PeerNVLinkProbePairCount is the number of GPU peer pairs where NVML returned
	// a usable P2P-over-NVLink status. This mirrors `nvidia-smi topo -p2p n`.
	PeerNVLinkProbePairCount int `json:"peer_nvlink_probe_pair_count,omitempty"`

	// PeerNVLinkExpectedPairCount is the total number of unique GPU peer pairs GPUD
	// tried to inspect. Comparing this with PeerNVLinkProbePairCount lets the
	// health evaluator distinguish a confirmed all-NS matrix from partial data.
	PeerNVLinkExpectedPairCount int `json:"peer_nvlink_expected_pair_count,omitempty"`

	// PeerNVLinkOKPairCount is the number of peer pairs that report NVLink P2P OK.
	PeerNVLinkOKPairCount int `json:"peer_nvlink_ok_pair_count,omitempty"`

	// PeerNVLinkOKGPUUUIDs lists GPUs that have at least one peer with NVLink P2P OK.
	PeerNVLinkOKGPUUUIDs []string `json:"peer_nvlink_ok_gpu_uuids,omitempty"`

	// PeerNVLinkObservedStatusCodes lists the distinct peer NVLink P2P status codes
	// observed across probed GPU pairs (e.g. OK, NS, TNS).
	PeerNVLinkObservedStatusCodes []string `json:"peer_nvlink_observed_status_codes,omitempty"`

	// SystemExpectedNVLink reports whether this node looks like a multi-GPU NVIDIA
	// host where GPUD expects an NVLink fabric to exist by default. This lets the
	// component catch obvious topology failures even when no explicit threshold is
	// configured.
	SystemExpectedNVLink bool `json:"system_expected_nvlink,omitempty"`

	// timestamp of the last check
	ts time.Time
	// error from the last check
	err error

	// tracks the healthy evaluation result of the last check
	health apiv1.HealthStateType
	// tracks the suggested actions for the last check
	suggestedActions *apiv1.SuggestedActions
	// tracks the reason of the last check
	reason string
}

func (cr *checkResult) hasCompletePeerNVLinkProbeCoverage() bool {
	if cr == nil || cr.PeerNVLinkExpectedPairCount == 0 {
		return false
	}
	return cr.PeerNVLinkProbePairCount == cr.PeerNVLinkExpectedPairCount
}

func (cr *checkResult) missingPeerNVLinkProbePairCount() int {
	if cr == nil || cr.PeerNVLinkExpectedPairCount <= cr.PeerNVLinkProbePairCount {
		return 0
	}
	return cr.PeerNVLinkExpectedPairCount - cr.PeerNVLinkProbePairCount
}

func (cr *checkResult) hasPeerNVLinkP2PFailure() bool {
	if cr == nil {
		return false
	}
	return cr.SystemExpectedNVLink &&
		len(cr.NVLinks) > 1 &&
		cr.PeerNVLinkProbePairCount > 0 &&
		cr.PeerNVLinkOKPairCount == 0
}

func (cr *checkResult) peerNVLinkStatusesSuggestReboot() bool {
	if cr == nil || len(cr.PeerNVLinkObservedStatusCodes) == 0 {
		return false
	}
	for _, statusCode := range cr.PeerNVLinkObservedStatusCodes {
		switch statusCode {
		case p2pStatusChipsetNotSupported,
			p2pStatusGPUNotSupported,
			p2pStatusTopologyNotSupported,
			p2pStatusDisabledByRegkey,
			p2pStatusNotSupported:
			continue
		default:
			return true
		}
	}
	return false
}

func (cr *checkResult) ComponentName() string {
	return Name
}

func (cr *checkResult) String() string {
	if cr == nil {
		return ""
	}
	kmsgSummary := ""
	if cr.KmsgScanned {
		kmsgSummary = fmt.Sprintf("matched %d kmsg(s)", len(cr.MatchedKmsgs))
	}
	if len(cr.NVLinks) == 0 {
		if kmsgSummary != "" {
			return kmsgSummary
		}
		return "no data"
	}

	buf := bytes.NewBuffer(nil)
	table := tablewriter.NewWriter(buf)
	table.SetAlignment(tablewriter.ALIGN_CENTER)
	headers := []string{"GPU UUID", "GPU Bus ID", "NVLink Enabled", "NVLink Supported"}
	peerNVLinkOKGPUUUIDs := make(map[string]struct{}, len(cr.PeerNVLinkOKGPUUUIDs))
	for _, uuid := range cr.PeerNVLinkOKGPUUUIDs {
		peerNVLinkOKGPUUUIDs[uuid] = struct{}{}
	}
	includePeerColumn := cr.PeerNVLinkProbePairCount > 0
	if includePeerColumn {
		headers = append(headers, "NVLink P2P OK")
	}
	table.SetHeader(headers)
	for _, nvlink := range cr.NVLinks {
		featureEnabled := nvlink.Supported && len(nvlink.States) > 0 && nvlink.States.AllFeatureEnabled()
		row := []string{
			nvlink.UUID,
			nvlink.BusID,
			fmt.Sprintf("%t", featureEnabled),
			fmt.Sprintf("%t", nvlink.Supported),
		}
		if includePeerColumn {
			_, ok := peerNVLinkOKGPUUUIDs[nvlink.UUID]
			row = append(row, fmt.Sprintf("%t", ok))
		}
		table.Append(row)
	}
	table.Render()

	if kmsgSummary != "" {
		return kmsgSummary + "\n\n" + buf.String()
	}
	return buf.String()
}

func (cr *checkResult) Summary() string {
	if cr == nil {
		return ""
	}
	if cr.KmsgScanned {
		return "scanned kmsg(s); " + cr.reason
	}
	return cr.reason
}

func (cr *checkResult) HealthStateType() apiv1.HealthStateType {
	if cr == nil {
		return ""
	}
	return cr.health
}

func (cr *checkResult) getSuggestedActions() *apiv1.SuggestedActions {
	if cr == nil {
		return nil
	}
	return cr.suggestedActions
}

func (cr *checkResult) getError() string {
	if cr == nil || cr.err == nil {
		return ""
	}
	return cr.err.Error()
}

func (cr *checkResult) HealthStates() apiv1.HealthStates {
	if cr == nil {
		return apiv1.HealthStates{
			{
				Time:      metav1.NewTime(time.Now().UTC()),
				Component: Name,
				Name:      Name,
				Health:    apiv1.HealthStateTypeHealthy,
				Reason:    "no data yet",
			},
		}
	}

	state := apiv1.HealthState{
		Time:             metav1.NewTime(cr.ts),
		Component:        Name,
		Name:             Name,
		Reason:           cr.reason,
		SuggestedActions: cr.getSuggestedActions(),
		Error:            cr.getError(),
		Health:           cr.health,
	}

	// propagate suggested actions to health state if present
	if cr.suggestedActions != nil {
		state.SuggestedActions = cr.suggestedActions
	}

	if len(cr.NVLinks) > 0 {
		b, _ := json.Marshal(cr)
		state.ExtraInfo = map[string]string{"data": string(b)}
	}
	return apiv1.HealthStates{state}
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
