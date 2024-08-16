package nvlink

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"

	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_nvlink"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	featureEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "feature_enabled",
			Help:      "tracks the NVLink feature enabled (aggregated for all links per GPU)",
		},
		[]string{"gpu_id"},
	)
	featureEnabledAverager = components_metrics.NewNoOpAverager()

	replayErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "replay_errors",
			Help:      "tracks the relay errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{"gpu_id"},
	)
	replayErrorsAverager = components_metrics.NewNoOpAverager()

	recoveryErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "recovery_errors",
			Help:      "tracks the recovery errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{"gpu_id"},
	)
	recoveryErrorsAverager = components_metrics.NewNoOpAverager()

	crcErrors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "crc_errors",
			Help:      "tracks the CRC errors in NVLink (aggregated for all links per GPU)",
		},
		[]string{"gpu_id"},
	)
	crcErrorsAverager = components_metrics.NewNoOpAverager()

	txBytesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "tx_bytes_total",
			Help:      "tracks the total number of bytes transmitted (cumulative) (aggregated for all links per GPU)",
		},
		[]string{"gpu_id"},
	)
	txBytesDelta = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "tx_bytes_delta",
			Help:      "tracks the number of bytes transmitted since the last collection (aggregated for all links per GPU)",
		},
		[]string{"gpu_id"},
	)
	txBytesAverager = components_metrics.NewNoOpAverager()

	rxBytesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "rx_bytes_total",
			Help:      "tracks the total number of bytes received (cumulative) (aggregated for all links per GPU)",
		},
		[]string{"gpu_id"},
	)
	rxBytesDelta = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "rx_bytes_delta",
			Help:      "tracks the number of bytes received since the last collection (aggregated for all links per GPU)",
		},
		[]string{"gpu_id"},
	)
	rxBytesAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(db *sql.DB, tableName string) {
	featureEnabledAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_feature_enabled")
	replayErrorsAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_replay_errors")
	recoveryErrorsAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_recovery_errors")
	crcErrorsAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_crc_errors")
	rxBytesAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_rx_bytes")
	txBytesAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_tx_bytes")
}

func ReadFeatureEnabled(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return featureEnabledAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadReplayErrors(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return replayErrorsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadRecoveryErrors(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return recoveryErrorsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadCRCErrors(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return crcErrorsAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadRxBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return rxBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadTxBytes(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return txBytesAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetFeatureEnabled(ctx context.Context, gpuID string, enabled bool, currentTime time.Time) error {
	v := float64(0)
	if enabled {
		v = float64(1)
	}
	featureEnabled.WithLabelValues(gpuID).Set(v)

	if err := featureEnabledAverager.Observe(
		ctx,
		v,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetReplayErrors(ctx context.Context, gpuID string, errors uint64, currentTime time.Time) error {
	replayErrors.WithLabelValues(gpuID).Set(float64(errors))

	if err := replayErrorsAverager.Observe(
		ctx,
		float64(errors),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetRecoveryErrors(ctx context.Context, gpuID string, errors uint64, currentTime time.Time) error {
	recoveryErrors.WithLabelValues(gpuID).Set(float64(errors))

	if err := recoveryErrorsAverager.Observe(
		ctx,
		float64(errors),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetCRCErrors(ctx context.Context, gpuID string, errors uint64, currentTime time.Time) error {
	crcErrors.WithLabelValues(gpuID).Set(float64(errors))

	if err := crcErrorsAverager.Observe(
		ctx,
		float64(errors),
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetTxBytes(ctx context.Context, gpuID string, bytes float64, currentTime time.Time) error {
	txBytesTotal.WithLabelValues(gpuID).Set(bytes)

	last, ok, err := txBytesAverager.Last(ctx, components_metrics.WithMetricSecondaryName(gpuID))
	if err != nil {
		return err
	}

	var v float64
	if ok { // previous value found, observe with delta
		v = bytes - last
	} else { // very first observe, just observe with the absolute value
		v = bytes
	}
	txBytesDelta.WithLabelValues(gpuID).Set(v)

	if err := txBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func SetRxBytes(ctx context.Context, gpuID string, bytes float64, currentTime time.Time) error {
	rxBytesTotal.WithLabelValues(gpuID).Set(bytes)

	last, ok, err := rxBytesAverager.Last(ctx, components_metrics.WithMetricSecondaryName(gpuID))
	if err != nil {
		return err
	}

	var v float64
	if ok { // previous value found, observe with delta
		v = bytes - last
	} else { // very first observe, just observe with the absolute value
		v = bytes
	}
	rxBytesDelta.WithLabelValues(gpuID).Set(v)

	if err := rxBytesAverager.Observe(
		ctx,
		bytes,
		components_metrics.WithCurrentTime(currentTime),
		components_metrics.WithMetricSecondaryName(gpuID),
	); err != nil {
		return err
	}

	return nil
}

func Register(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	InitAveragers(db, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(featureEnabled); err != nil {
		return err
	}
	if err := reg.Register(replayErrors); err != nil {
		return err
	}
	if err := reg.Register(recoveryErrors); err != nil {
		return err
	}
	if err := reg.Register(crcErrors); err != nil {
		return err
	}
	if err := reg.Register(txBytesTotal); err != nil {
		return err
	}
	if err := reg.Register(txBytesDelta); err != nil {
		return err
	}
	if err := reg.Register(rxBytesTotal); err != nil {
		return err
	}
	if err := reg.Register(rxBytesDelta); err != nil {
		return err
	}
	return nil
}
