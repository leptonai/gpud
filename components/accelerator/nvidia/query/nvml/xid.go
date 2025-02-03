package nvml

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/leptonai/gpud/components"
	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/components/common"
	"github.com/leptonai/gpud/log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type XidEvent struct {
	// Time is the time the metrics were collected.
	Time metav1.Time `json:"time"`
	// The duration of the sample.
	SampleDuration metav1.Duration `json:"sample_duration"`

	DeviceUUID string `json:"device_uuid"`

	Xid uint64 `json:"xid"`

	NVMLEventType                  uint64 `json:"nvml_event_type"`
	NVMLEventTypeSingleBitEccError bool   `json:"nvml_event_type_single_bit_ecc_error"`
	NVMLEventTypeDoubleBitEccError bool   `json:"nvml_event_type_double_bit_ecc_error"`
	NVMLEventTypePState            bool   `json:"nvml_event_type_p_state"`
	NVMLEventTypeXidCriticalError  bool   `json:"nvml_event_type_xid_critical_error"`
	NVMLEventTypeClock             bool   `json:"nvml_event_type_clock"`
	NVMLEventTypePowerSourceChange bool   `json:"nvml_event_type_power_source_change"`
	NVMLEventMigConfigChange       bool   `json:"nvml_event_type_mig_config_change"`

	Detail *nvidia_query_xid.Detail `json:"detail"`

	Message string `json:"message,omitempty"`

	// Set if any error happens during NVML calls.
	Error error `json:"error,omitempty"`
}

func (ev *XidEvent) JSON() ([]byte, error) {
	return json.Marshal(ev)
}

func (ev *XidEvent) YAML() ([]byte, error) {
	return yaml.Marshal(ev)
}

func (inst *instance) XidErrorSupported() bool {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	return inst.xidErrorSupported
}

func (inst *instance) RecvXidEvents() <-chan *XidEvent {
	inst.mu.RLock()
	defer inst.mu.RUnlock()

	if inst.nvmlLib == nil {
		return nil
	}

	return inst.xidEventCh
}

// k8s-device-plugin uses only nvml.EventTypeXidCriticalError | nvml.EventTypeDoubleBitEccError | nvml.EventTypeSingleBitEccError
// ref. https://github.com/NVIDIA/k8s-device-plugin/blob/main/internal/rm/health.go
//
// we want to cover all events and decide the criticality by ourselves
// ref. https://github.com/NVIDIA/go-nvml/blob/main/gen/nvml/nvml.h
const defaultXidEventMask = uint64(nvml.EventTypeAll)

// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlEvents.html#group__nvmlEvents
func (inst *instance) pollXidEvents() {
	log.Logger.Debugw("polling xid events")

	for {
		select {
		case <-inst.rootCtx.Done():
			return
		default:
		}

		// ok to for-loop with infinite 5-second retry
		// because the below wait call blocks 5-second anyways
		// and we do not want to miss the events between retries
		// the event is only sent to the "xidEventCh" channel
		// if it's an Xid event thus safe to retry in the for-loop

		// waits 5 seconds
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlEvents.html#group__nvmlEvents
		e, ret := inst.xidEventSet.Wait(5000)

		if IsNotSupportError(ret) {
			log.Logger.Warnw("xid events not supported -- skipping", "error", nvml.ErrorString(ret))
			return
		}

		if ret == nvml.ERROR_TIMEOUT {
			log.Logger.Debugw("no event found in wait (timeout) -- retrying...", "error", nvml.ErrorString(ret))
			continue
		}

		if ret != nvml.SUCCESS {
			log.Logger.Warnw("notifying event set wait failure", "error", nvml.ErrorString(ret))
			select {
			case <-inst.rootCtx.Done():
				return

			case inst.xidEventCh <- &XidEvent{
				Time:    metav1.Time{Time: time.Now().UTC()},
				Message: "event set wait returned non-success",
				Error:   fmt.Errorf("event set wait failed: %v", nvml.ErrorString(ret)),
			}:
				log.Logger.Warnw("notified event set wait failure", "error", nvml.ErrorString(ret))
			default:
				log.Logger.Warnw("xid event channel is full -- skipping sending wait failure event")
			}

			continue
		}

		xid := e.EventData

		if xid == 0 {
			log.Logger.Debugw("received xid 0 as an event -- skipping", "return", ret)
			continue
		}

		msg := fmt.Sprintf("received event with a known xid: %d", xid)
		xidDetail, ok := nvidia_query_xid.GetDetail(int(xid))
		if !ok {
			msg = fmt.Sprintf("received event but xid unknown: %d", xid)
		}
		log.Logger.Warnw("detected xid event", "xid", xid, "message", msg)

		var deviceUUID string
		var deviceUUIDErr error
		deviceUUID, ret = e.Device.GetUUID()
		if IsNotSupportError(ret) {
			// "If we cannot reliably determine the device UUID, we mark all devices as unhealthy."
			// ref. nvidia/k8s-device-plugin/internal/rm/health.go
			deviceUUIDErr = fmt.Errorf("failed to get device UUID: %v", nvml.ErrorString(ret))
		}

		now := time.Now().UTC()

		event := components.Event{
			Time:    metav1.Time{Time: now},
			Name:    "error_xid",
			Type:    common.EventTypeUnknown,
			Message: msg,
		}
		if xidDetail != nil {
			event.Type = xidDetail.EventType
			event.SuggestedActions = xidDetail.SuggestedActionsByGPUd
		}

		xidEvent := &XidEvent{
			Time:           metav1.Time{Time: now},
			SampleDuration: metav1.Duration{Duration: 5 * time.Second},

			DeviceUUID: deviceUUID,
			Xid:        xid,

			NVMLEventType:                  e.EventType,
			NVMLEventTypeSingleBitEccError: e.EventType == nvml.EventTypeSingleBitEccError,
			NVMLEventTypeDoubleBitEccError: e.EventType == nvml.EventTypeDoubleBitEccError,
			NVMLEventTypePState:            e.EventType == nvml.EventTypePState,
			NVMLEventTypeXidCriticalError:  e.EventType == nvml.EventTypeXidCriticalError,
			NVMLEventTypeClock:             e.EventType == nvml.EventTypeClock,
			NVMLEventTypePowerSourceChange: e.EventType == nvml.EventTypePowerSourceChange,
			NVMLEventMigConfigChange:       e.EventType == nvml.EventMigConfigChange,

			Detail: xidDetail,

			Message: msg,

			Error: deviceUUIDErr,
		}
		eb, err := xidEvent.JSON()
		if err != nil {
			log.Logger.Errorw("failed to marshal xid event", "error", err)
			continue
		}
		event.ExtraInfo = map[string]string{
			"xid_event": string(eb),
		}

		// no need to check duplicate entries, assuming nvml event poller does not return old events
		ctx, cancel := context.WithTimeout(inst.rootCtx, 10*time.Second)
		werr := inst.xidEventsStore.Insert(ctx, event)
		cancel()
		if werr != nil {
			log.Logger.Errorw("failed to insert xid event into events store", "error", werr)
		}

		select {
		case <-inst.rootCtx.Done():
			return
		case inst.xidEventCh <- xidEvent:
			log.Logger.Warnw("notified xid event", "event", event)
		default:
			log.Logger.Warnw("xid event channel is full, skipping event")
		}
	}
}
