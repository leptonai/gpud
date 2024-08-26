package nvml

import (
	"fmt"

	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
	"github.com/leptonai/gpud/log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"sigs.k8s.io/yaml"
)

type XidEvent struct {
	EventType uint64 `json:"event_type"`

	Xid              uint64 `json:"xid"`
	XidCriticalError bool   `json:"xid_critical_error"`

	Detail *nvidia_query_xid.Detail `json:"detail,omitempty"`

	Message string `json:"message,omitempty"`

	// Set if any error happens during NVML calls.
	Error error `json:"error,omitempty"`
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

const defaultXidEventMask = uint64(nvml.EventTypeXidCriticalError | nvml.EventTypeDoubleBitEccError | nvml.EventTypeSingleBitEccError)

// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlEvents.html#group__nvmlEvents
func (inst *instance) pollXidEvents() {
	log.Logger.Debugw("polling xid events")
	for {
		select {
		case <-inst.rootCtx.Done():
			return
		default:
		}

		// waits 5 seconds
		// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlEvents.html#group__nvmlEvents
		e, ret := inst.xidEventSet.Wait(5000)

		if ret == nvml.ERROR_TIMEOUT {
			log.Logger.Debugw("no event found in wait (timeout) -- retrying...", "error", nvml.ErrorString(ret))
			continue
		}

		if ret != nvml.SUCCESS {
			select {
			case <-inst.rootCtx.Done():
				return

			case inst.xidEventCh <- &XidEvent{
				Message: "event set wait returned non-success",
				Error:   fmt.Errorf("event set wait failed: %v", nvml.ErrorString(ret)),
			}:
				log.Logger.Debugw("event set wait failure notified", "error", nvml.ErrorString(ret))
			default:
				log.Logger.Debugw("xid event channel is full, skipping event")
			}

			continue
		}

		xid := e.EventData

		var xidDetail *nvidia_query_xid.Detail
		msg := "received event but xid unknown"
		if xid > 0 {
			var ok bool
			xidDetail, ok = nvidia_query_xid.GetDetail(int(xid))
			if ok {
				msg = "received event with a known xid"
			}
		}

		event := &XidEvent{
			EventType: e.EventType,

			Xid:              xid,
			XidCriticalError: e.EventType == nvml.EventTypeXidCriticalError,

			Detail: xidDetail,

			Message: msg,
		}
		select {
		case <-inst.rootCtx.Done():
			return
		case inst.xidEventCh <- event:
		default:
			log.Logger.Debugw("xid event channel is full, skipping event")
		}
	}
}
