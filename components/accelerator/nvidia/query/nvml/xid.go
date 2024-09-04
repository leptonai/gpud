package nvml

import (
	"fmt"
	"time"

	nvidia_query_xid "github.com/leptonai/gpud/components/accelerator/nvidia/query/xid"
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

	EventType uint64 `json:"event_type"`

	DeviceUUID       string `json:"device_uuid"`
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

	ticker := time.NewTicker(1)
	defer ticker.Stop()

	for {
		select {
		case <-inst.rootCtx.Done():
			return
		case <-ticker.C:
			ticker.Reset(inst.xidPollInterval)
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
				Time:    metav1.Time{Time: time.Now().UTC()},
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

		var deviceUUID string
		var deviceUUIDErr error
		deviceUUID, ret = e.Device.GetUUID()
		if ret != nvml.SUCCESS {
			// "If we cannot reliably determine the device UUID, we mark all devices as unhealthy."
			// ref. nvidia/k8s-device-plugin/internal/rm/health.go
			deviceUUIDErr = fmt.Errorf("failed to get device UUID: %v", nvml.ErrorString(ret))
		}

		event := &XidEvent{
			Time:           metav1.Time{Time: time.Now().UTC()},
			SampleDuration: metav1.Duration{Duration: 5 * time.Second},

			EventType: e.EventType,

			DeviceUUID:       deviceUUID,
			Xid:              xid,
			XidCriticalError: e.EventType == nvml.EventTypeXidCriticalError,

			Detail: xidDetail,

			Message: msg,

			Error: deviceUUIDErr,
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
