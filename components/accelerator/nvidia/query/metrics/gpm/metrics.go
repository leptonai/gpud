// Package gpm provides the NVIDIA GPM metrics collection and reporting.
package gpm

import (
	"context"
	"database/sql"
	"time"

	components_metrics "github.com/leptonai/gpud/components/metrics"
	components_metrics_state "github.com/leptonai/gpud/components/metrics/state"
	"github.com/leptonai/gpud/log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/prometheus/client_golang/prometheus"
)

const SubSystem = "accelerator_nvidia_gpm"

var (
	lastUpdateUnixSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "last_update_unix_seconds",
			Help:      "tracks the last update time in unix seconds",
		},
	)

	// gpuSMOccupancyPercent is the percentage of warps that were active vs theoretical maximum (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_SM_OCCUPANCY or DCGM_FI_PROF_SM_OCCUPANCY in DCGM exporter.
	// It's the ratio of number of warps resident on an SM.
	// It's the number of resident as a ratio of the theoretical maximum number of warps per elapsed cycle.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlGpmStructs.html#group__nvmlGpmStructs_1g168f5f2704ec9871110d22aa1879aec0
	gpuSMOccupancyPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "gpu_sm_occupancy_percent",
			Help:      "tracks the current GPU SM occupancy, as a percentage of warps that were active vs theoretical maximum",
		},
		[]string{"gpu_id"},
	)
	gpuSMOccupancyPercentAverager = components_metrics.NewNoOpAverager()

	// gpuIntUtilPercent is the percentage of time the GPU's SMs were doing integer operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_INTEGER_UTIL.
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10642C5-L10642C33
	gpuIntUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_int_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing integer operations",
	}, []string{"gpu_id"})
	gpuIntUtilPercentAverager = components_metrics.NewNoOpAverager()

	// gpuAnyTensorUtilPercent is the percentage of time the GPU's SMs were doing ANY tensor operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_ANY_TENSOR_UTIL
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10643
	gpuAnyTensorUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_any_tensor_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing ANY tensor operations",
	}, []string{"gpu_id"})
	gpuAnyTensorUtilPercentAverager = components_metrics.NewNoOpAverager()

	// gpuDFMATensorUtilPercent is the percentage of time the GPU's SMs were doing DFMA tensor operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_DFMA_TENSOR_UTIL.
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10644
	gpuDFMATensorUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_dfma_tensor_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing DFMA tensor operations",
	}, []string{"gpu_id"})
	gpuDFMATensorUtilPercentAverager = components_metrics.NewNoOpAverager()

	// gpuHMMATensorUtilPercent is the percentage of time the GPU's SMs were doing HMMA tensor operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_HMMA_TENSOR_UTIL.
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10645
	gpuHMMATensorUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_hmma_tensor_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing HMMA tensor operations",
	}, []string{"gpu_id"})
	gpuHMMATensorUtilPercentAverager = components_metrics.NewNoOpAverager()

	// gpuIMMATensorUtilPercent is the percentage of time the GPU's SMs were doing IMMA tensor operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_IMMA_TENSOR_UTIL.
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10646
	gpuIMMATensorUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_imma_tensor_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing IMMA tensor operations",
	}, []string{"gpu_id"})
	gpuIMMATensorUtilPercentAverager = components_metrics.NewNoOpAverager()

	// gpuFp64UtilPercent is the percentage of time the GPU's SMs were doing non-tensor FP64 math (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_FP64_UTIL
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10648
	gpuFp64UtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_fp64_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing non-tensor FP64 math",
	}, []string{"gpu_id"})
	gpuFp64UtilPercentAverager = components_metrics.NewNoOpAverager()

	// gpuFp32UtilPercent is the percentage of time the GPU's SMs were doing non-tensor FP32 math (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_FP32_UTIL
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10649
	gpuFp32UtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_fp32_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing non-tensor FP32 math",
	}, []string{"gpu_id"})
	gpuFp32UtilPercentAverager = components_metrics.NewNoOpAverager()

	// gpuFp16UtilPercent is the percentage of time the GPU's SMs were doing non-tensor FP16 math (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_FP16_UTIL
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10650
	gpuFp16UtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_fp16_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing non-tensor FP16 math",
	}, []string{"gpu_id"})
	gpuFp16UtilPercentAverager = components_metrics.NewNoOpAverager()
)

func InitAveragers(db *sql.DB, tableName string) {
	gpuSMOccupancyPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_sm_occupancy_percent")
	gpuIntUtilPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_int_util_percent")
	gpuAnyTensorUtilPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_any_tensor_util_percent")
	gpuDFMATensorUtilPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_dfma_tensor_util_percent")
	gpuHMMATensorUtilPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_hmma_tensor_util_percent")
	gpuIMMATensorUtilPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_imma_tensor_util_percent")
	gpuFp64UtilPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_fp64_util_percent")
	gpuFp32UtilPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_fp32_util_percent")
	gpuFp16UtilPercentAverager = components_metrics.NewAverager(db, tableName, SubSystem+"_gpu_fp16_util_percent")
}

func ReadGPUSMOccupancyPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuSMOccupancyPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadGPUIntUtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuIntUtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadGPUAnyTensorUtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuAnyTensorUtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadGPUDFMATensorUtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuDFMATensorUtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadGPUHMMATensorUtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuHMMATensorUtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadGPUIMMATensorUtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuIMMATensorUtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadGPUFp64UtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuFp64UtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadGPUFp32UtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuFp32UtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func ReadGPUFp16UtilPercents(ctx context.Context, since time.Time) (components_metrics_state.Metrics, error) {
	return gpuFp16UtilPercentAverager.Read(ctx, components_metrics.WithSince(since))
}

func SetLastUpdateUnixSeconds(unixSeconds float64) {
	lastUpdateUnixSeconds.Set(unixSeconds)
}

func SetGPUUtilPercent(ctx context.Context, metricID nvml.GpmMetricId, gpuID string, pct float64, currentTime time.Time) error {
	switch metricID {
	case nvml.GPM_METRIC_SM_OCCUPANCY:
		gpuSMOccupancyPercent.WithLabelValues(gpuID).Set(pct)

		if err := gpuSMOccupancyPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	case nvml.GPM_METRIC_INTEGER_UTIL:
		gpuIntUtilPercent.WithLabelValues(gpuID).Set(pct)

		if err := gpuIntUtilPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	case nvml.GPM_METRIC_ANY_TENSOR_UTIL:
		gpuAnyTensorUtilPercent.WithLabelValues(gpuID).Set(pct)
		if err := gpuAnyTensorUtilPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	case nvml.GPM_METRIC_DFMA_TENSOR_UTIL:
		gpuDFMATensorUtilPercent.WithLabelValues(gpuID).Set(pct)
		if err := gpuDFMATensorUtilPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	case nvml.GPM_METRIC_HMMA_TENSOR_UTIL:
		gpuHMMATensorUtilPercent.WithLabelValues(gpuID).Set(pct)
		if err := gpuHMMATensorUtilPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	case nvml.GPM_METRIC_IMMA_TENSOR_UTIL:
		gpuIMMATensorUtilPercent.WithLabelValues(gpuID).Set(pct)
		if err := gpuHMMATensorUtilPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	case nvml.GPM_METRIC_FP64_UTIL:
		gpuFp64UtilPercent.WithLabelValues(gpuID).Set(pct)
		if err := gpuFp64UtilPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	case nvml.GPM_METRIC_FP32_UTIL:
		gpuFp32UtilPercent.WithLabelValues(gpuID).Set(pct)
		if err := gpuFp32UtilPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	case nvml.GPM_METRIC_FP16_UTIL:
		gpuFp16UtilPercent.WithLabelValues(gpuID).Set(pct)
		if err := gpuFp16UtilPercentAverager.Observe(
			ctx,
			pct,
			components_metrics.WithCurrentTime(currentTime),
			components_metrics.WithMetricSecondaryName(gpuID),
		); err != nil {
			return err
		}
	default:
		log.Logger.Warnw("unsupported gpm metric id", "id", metricID)
	}

	return nil
}

func Register(reg *prometheus.Registry, db *sql.DB, tableName string) error {
	InitAveragers(db, tableName)

	if err := reg.Register(lastUpdateUnixSeconds); err != nil {
		return err
	}
	if err := reg.Register(gpuSMOccupancyPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuIntUtilPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuAnyTensorUtilPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuDFMATensorUtilPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuHMMATensorUtilPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuIMMATensorUtilPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuFp64UtilPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuFp32UtilPercent); err != nil {
		return err
	}
	if err := reg.Register(gpuFp16UtilPercent); err != nil {
		return err
	}
	return nil
}
