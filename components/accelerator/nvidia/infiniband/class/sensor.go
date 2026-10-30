// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// modified from https://github.com/prometheus/procfs/blob/master/sysfs/class_infiniband.go

// Package class adapts health-validation's sensor observations to GPUd's stored port data.
package class

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/dsx-ai-factory/health-validation/collect"
	"github.com/dsx-ai-factory/health-validation/collect/observation"
	ibsensor "github.com/dsx-ai-factory/health-validation/collect/sensors/infiniband"
	ibsignals "github.com/dsx-ai-factory/health-validation/collect/signals/infiniband"
	ibsource "github.com/dsx-ai-factory/health-validation/collect/source/infiniband"
)

const DefaultRootDir = ibsource.DefaultRoot

// Reader is this component's sensor shim. It retains one sensor across checks
// and converts observations to GPUd device data without reading sysfs itself.
type Reader struct {
	sensor collect.Sensor
}

func New(rootDir string, excludedDevices ...string) *Reader {
	if rootDir == "" {
		rootDir = DefaultRootDir
	}
	files := os.DirFS(rootDir)
	return &Reader{sensor: ibsensor.New(ibsource.New(files, excludedDevices...))}
}

// Read collects through the public sensor. Non-InfiniBand ports are omitted by
// the sensor; GPUd owns the output schema and health interpretation.
func (r *Reader) Read(ctx context.Context) (Devices, error) {
	now := time.Now()
	cycle, err := collect.NewCycle("gpud.infiniband."+now.Format(time.RFC3339Nano), now)
	if err != nil {
		return nil, err
	}
	batch, err := r.sensor.Measure(ctx, collect.WithCycle(cycle))
	if err != nil {
		return nil, err
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return devicesFromObservations(batch)
}

// devicesFromObservations is a pure conversion: missing counters remain nil,
// and required-field collection failures never become healthy zero values.
func devicesFromObservations(batch *observation.ObservationBatch) (Devices, error) {
	if batch == nil {
		return nil, fmt.Errorf("InfiniBand sensor returned no observation batch")
	}
	devices := make(Devices, 0)
	for _, reading := range batch.Observations {
		if reading == nil {
			return nil, fmt.Errorf("InfiniBand sensor returned a nil observation")
		}
		if reading.GetCollectionError() != nil {
			if err := readingError(reading); err != nil {
				return nil, err
			}
			continue
		}
		if missing := reading.GetNoValue(); missing != nil {
			if isCounter(reading.SignalId) && missing.Reason == observation.NoValueReasonNotSupported {
				continue
			}
			return nil, fmt.Errorf("InfiniBand signal %s has no value: %s", reading.SignalId, missing.Detail)
		}
		if reading.GetValue() == nil {
			return nil, fmt.Errorf("InfiniBand signal %s has no outcome", reading.SignalId)
		}
		portRef := reading.Target.GetInfiniBandPort()
		name := reading.Target.GetInfiniBandDevice().GetName()
		if portRef != nil {
			name = portRef.GetDevice().GetName()
		}
		if len(devices) == 0 || devices[len(devices)-1].Name != name {
			devices = append(devices, Device{Name: name, Ports: make([]Port, 0)})
		}
		device := &devices[len(devices)-1]
		switch reading.SignalId {
		case ibsignals.FirmwareVersion.ID():
			device.FirmwareVersion = reading.GetValue().GetStringValue()
		case ibsignals.BoardID.ID():
			device.BoardID = reading.GetValue().GetStringValue()
		case ibsignals.HCAType.ID():
			device.HCAType = reading.GetValue().GetStringValue()
		default:
			if portRef == nil {
				return nil, fmt.Errorf("InfiniBand signal %s has no port target", reading.SignalId)
			}
			number := uint(portRef.GetPort())
			if len(device.Ports) == 0 || device.Ports[len(device.Ports)-1].Port != number {
				device.Ports = append(device.Ports, Port{Name: name, Port: number})
			}
			port := &device.Ports[len(device.Ports)-1]
			if err := setPortValue(port, reading); err != nil {
				return nil, err
			}
		}
	}
	return devices, nil
}

func isCounter(signalID string) bool {
	return strings.HasPrefix(signalID, "infiniband.counters.") || strings.HasPrefix(signalID, "infiniband.hw_counters.")
}

func readingError(reading *observation.Observation) error {
	failure := reading.GetCollectionError()
	// Targetless failures describe the whole collection, never an optional field.
	if reading.Target.GetInfiniBandDevice() != nil {
		switch reading.SignalId {
		case ibsignals.BoardID.ID(), ibsignals.HCAType.ID():
			if failure.Category == observation.CollectionErrorCategoryUnavailable {
				return nil
			}
		}
	}
	dir := path.Base(path.Dir(failure.Operation))
	if reading.Target.GetInfiniBandPort() != nil && isCounter(reading.SignalId) && (dir == "counters" || dir == "hw_counters") {
		switch failure.Category {
		case observation.CollectionErrorCategoryUnavailable, observation.CollectionErrorCategoryPermissionDenied, observation.CollectionErrorCategoryUnsupported:
			return nil
		}
	}
	return fmt.Errorf("collect InfiniBand %s (%s): %s", reading.SignalId, failure.Operation, failure.Detail)
}

func setPortValue(port *Port, reading *observation.Observation) error {
	value := reading.GetValue()
	counter := uint64(value.GetIntValue())
	switch reading.SignalId {
	case ibsignals.LinkLayer.ID():
		port.LinkLayer = value.GetStringValue()
	case ibsignals.State.ID():
		port.StateID = uint(value.GetIntValue())
	case ibsignals.StateName.ID():
		port.State = value.GetStringValue()
	case ibsignals.PhysicalState.ID():
		port.PhysStateID = uint(value.GetIntValue())
	case ibsignals.PhysicalStateName.ID():
		port.PhysState = value.GetStringValue()
	case ibsignals.Rate.ID():
		port.Rate = uint64(value.GetIntValue())
	case ibsignals.RateGbps.ID():
		port.RateGBSec = value.GetDoubleValue()
	case ibsignals.ExcessiveBufferOverrunErrors.ID():
		port.Counters.ExcessiveBufferOverrunErrors = &counter
	case ibsignals.LinkDowned.ID():
		port.Counters.LinkDowned = &counter
	case ibsignals.LinkErrorRecovery.ID():
		port.Counters.LinkErrorRecovery = &counter
	case ibsignals.LocalLinkIntegrityErrors.ID():
		port.Counters.LocalLinkIntegrityErrors = &counter
	case ibsignals.MulticastRcvPackets.ID():
		port.Counters.MulticastRcvPackets = &counter
	case ibsignals.MulticastXmitPackets.ID():
		port.Counters.MulticastXmitPackets = &counter
	case ibsignals.PortRcvConstraintErrors.ID():
		port.Counters.PortRcvConstraintErrors = &counter
	case ibsignals.PortRcvData.ID():
		port.Counters.PortRcvData = &counter
	case ibsignals.PortRcvDiscards.ID():
		port.Counters.PortRcvDiscards = &counter
	case ibsignals.PortRcvErrors.ID():
		port.Counters.PortRcvErrors = &counter
	case ibsignals.PortRcvPackets.ID():
		port.Counters.PortRcvPackets = &counter
	case ibsignals.PortRcvRemotePhysicalErrors.ID():
		port.Counters.PortRcvRemotePhysicalErrors = &counter
	case ibsignals.PortRcvSwitchRelayErrors.ID():
		port.Counters.PortRcvSwitchRelayErrors = &counter
	case ibsignals.PortXmitConstraintErrors.ID():
		port.Counters.PortXmitConstraintErrors = &counter
	case ibsignals.PortXmitData.ID():
		port.Counters.PortXmitData = &counter
	case ibsignals.PortXmitDiscards.ID():
		port.Counters.PortXmitDiscards = &counter
	case ibsignals.PortXmitPackets.ID():
		port.Counters.PortXmitPackets = &counter
	case ibsignals.PortXmitWait.ID():
		port.Counters.PortXmitWait = &counter
	case ibsignals.SymbolError.ID():
		port.Counters.SymbolError = &counter
	case ibsignals.UnicastRcvPackets.ID():
		port.Counters.UnicastRcvPackets = &counter
	case ibsignals.UnicastXmitPackets.ID():
		port.Counters.UnicastXmitPackets = &counter
	case ibsignals.VL15Dropped.ID():
		port.Counters.VL15Dropped = &counter
	case ibsignals.HardwareDuplicateRequest.ID():
		port.HWCounters.DuplicateRequest = &counter
	case ibsignals.HardwareImpliedNakSeqErr.ID():
		port.HWCounters.ImpliedNakSeqErr = &counter
	case ibsignals.HardwareLifespan.ID():
		port.HWCounters.Lifespan = &counter
	case ibsignals.HardwareLocalAckTimeoutErr.ID():
		port.HWCounters.LocalAckTimeoutErr = &counter
	case ibsignals.HardwareNpCnpSent.ID():
		port.HWCounters.NpCnpSent = &counter
	case ibsignals.HardwareNpEcnMarkedRocePackets.ID():
		port.HWCounters.NpEcnMarkedRocePackets = &counter
	case ibsignals.HardwareOutOfBuffer.ID():
		port.HWCounters.OutOfBuffer = &counter
	case ibsignals.HardwareOutOfSequence.ID():
		port.HWCounters.OutOfSequence = &counter
	case ibsignals.HardwarePacketSeqErr.ID():
		port.HWCounters.PacketSeqErr = &counter
	case ibsignals.HardwareReqCqeError.ID():
		port.HWCounters.ReqCqeError = &counter
	case ibsignals.HardwareReqCqeFlushError.ID():
		port.HWCounters.ReqCqeFlushError = &counter
	case ibsignals.HardwareReqRemoteAccessErrors.ID():
		port.HWCounters.ReqRemoteAccessErrors = &counter
	case ibsignals.HardwareReqRemoteInvalidRequest.ID():
		port.HWCounters.ReqRemoteInvalidRequest = &counter
	case ibsignals.HardwareRespCqeError.ID():
		port.HWCounters.RespCqeError = &counter
	case ibsignals.HardwareRespCqeFlushError.ID():
		port.HWCounters.RespCqeFlushError = &counter
	case ibsignals.HardwareRespLocalLengthError.ID():
		port.HWCounters.RespLocalLengthError = &counter
	case ibsignals.HardwareRespRemoteAccessErrors.ID():
		port.HWCounters.RespRemoteAccessErrors = &counter
	case ibsignals.HardwareRnrNakRetryErr.ID():
		port.HWCounters.RnrNakRetryErr = &counter
	case ibsignals.HardwareRoceAdpRetrans.ID():
		port.HWCounters.RoceAdpRetrans = &counter
	case ibsignals.HardwareRoceAdpRetransTo.ID():
		port.HWCounters.RoceAdpRetransTo = &counter
	case ibsignals.HardwareRoceSlowRestart.ID():
		port.HWCounters.RoceSlowRestart = &counter
	case ibsignals.HardwareRoceSlowRestartCnps.ID():
		port.HWCounters.RoceSlowRestartCnps = &counter
	case ibsignals.HardwareRoceSlowRestartTrans.ID():
		port.HWCounters.RoceSlowRestartTrans = &counter
	case ibsignals.HardwareRpCnpHandled.ID():
		port.HWCounters.RpCnpHandled = &counter
	case ibsignals.HardwareRpCnpIgnored.ID():
		port.HWCounters.RpCnpIgnored = &counter
	case ibsignals.HardwareRxAtomicRequests.ID():
		port.HWCounters.RxAtomicRequests = &counter
	case ibsignals.HardwareRxDctConnect.ID():
		port.HWCounters.RxDctConnect = &counter
	case ibsignals.HardwareRxIcrcEncapsulated.ID():
		port.HWCounters.RxIcrcEncapsulated = &counter
	case ibsignals.HardwareRxReadRequests.ID():
		port.HWCounters.RxReadRequests = &counter
	case ibsignals.HardwareRxWriteRequests.ID():
		port.HWCounters.RxWriteRequests = &counter
	default:
		return fmt.Errorf("unsupported InfiniBand signal %s", reading.SignalId)
	}
	return nil
}
