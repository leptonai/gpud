// Package gpm provides the NVIDIA GPM metrics collection and reporting.
package gpm

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/leptonai/gpud/pkg/log"
	pkgmetrics "github.com/leptonai/gpud/pkg/metrics"
)

const SubSystem = "accelerator_nvidia_gpm"

var (
	componentLabel = prometheus.Labels{
		pkgmetrics.MetricComponentLabelKey: "accelerator-nvidia-gpm",
	}

	// metricGPUSMOccupancyPercent is the percentage of warps that were active vs theoretical maximum (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_SM_OCCUPANCY or DCGM_FI_PROF_SM_OCCUPANCY in DCGM exporter.
	// It's the ratio of number of warps resident on an SM.
	// It's the number of resident as a ratio of the theoretical maximum number of warps per elapsed cycle.
	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlGpmStructs.html#group__nvmlGpmStructs_1g168f5f2704ec9871110d22aa1879aec0
	metricGPUSMOccupancyPercent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "",
			Subsystem: SubSystem,
			Name:      "gpu_sm_occupancy_percent",
			Help:      "tracks the current GPU SM occupancy, as a percentage of warps that were active vs theoretical maximum",
		},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	// metricGPUIntUtilPercent is the percentage of time the GPU's SMs were doing integer operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_INTEGER_UTIL.
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10642C5-L10642C33
	metricGPUIntUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_int_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing integer operations",
	},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	// metricGPUAnyTensorUtilPercent is the percentage of time the GPU's SMs were doing ANY tensor operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_ANY_TENSOR_UTIL
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10643
	metricGPUAnyTensorUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_any_tensor_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing ANY tensor operations",
	},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	// metricGPUDFMATensorUtilPercent is the percentage of time the GPU's SMs were doing DFMA tensor operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_DFMA_TENSOR_UTIL.
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10644
	metricGPUDFMATensorUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_dfma_tensor_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing DFMA tensor operations",
	},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	// metricGPUHMMATensorUtilPercent is the percentage of time the GPU's SMs were doing HMMA tensor operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_HMMA_TENSOR_UTIL.
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10645
	metricGPUHMMATensorUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_hmma_tensor_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing HMMA tensor operations",
	},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	// metricGPUIMMATensorUtilPercent is the percentage of time the GPU's SMs were doing IMMA tensor operations (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_IMMA_TENSOR_UTIL.
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10646
	metricGPUIMMATensorUtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_imma_tensor_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing IMMA tensor operations",
	},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	// metricGPUFp64UtilPercent is the percentage of time the GPU's SMs were doing non-tensor FP64 math (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_FP64_UTIL
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10648
	metricGPUFp64UtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_fp64_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing non-tensor FP64 math",
	},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	// metricGPUFp32UtilPercent is the percentage of time the GPU's SMs were doing non-tensor FP32 math (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_FP32_UTIL
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10649
	metricGPUFp32UtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_fp32_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing non-tensor FP32 math",
	},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)

	// metricGPUFp16UtilPercent is the percentage of time the GPU's SMs were doing non-tensor FP16 math (0.0 - 100.0).
	// It's defined as NVML_GPM_METRIC_FP16_UTIL
	// ref. https://github.com/NVIDIA/go-nvml/blob/150a069935f8d725c37354faa051e3723e6444c0/gen/nvml/nvml.h#L10650
	metricGPUFp16UtilPercent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "",
		Subsystem: SubSystem,
		Name:      "gpu_fp16_util_percent",
		Help:      "tracks the percentage of time the GPU's SMs were doing non-tensor FP16 math",
	},
		[]string{pkgmetrics.MetricComponentLabelKey, "uuid"}, // label is GPU ID
	).MustCurryWith(componentLabel)
)

func recordGPMMetricByID(metricID nvml.GpmMetricId, gpuID string, pct float64) {
	switch metricID {
	case nvml.GPM_METRIC_SM_OCCUPANCY:
		metricGPUSMOccupancyPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	case nvml.GPM_METRIC_INTEGER_UTIL:
		metricGPUIntUtilPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	case nvml.GPM_METRIC_ANY_TENSOR_UTIL:
		metricGPUAnyTensorUtilPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	case nvml.GPM_METRIC_DFMA_TENSOR_UTIL:
		metricGPUDFMATensorUtilPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	case nvml.GPM_METRIC_HMMA_TENSOR_UTIL:
		metricGPUHMMATensorUtilPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	case nvml.GPM_METRIC_IMMA_TENSOR_UTIL:
		metricGPUIMMATensorUtilPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	case nvml.GPM_METRIC_FP64_UTIL:
		metricGPUFp64UtilPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	case nvml.GPM_METRIC_FP32_UTIL:
		metricGPUFp32UtilPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	case nvml.GPM_METRIC_FP16_UTIL:
		metricGPUFp16UtilPercent.With(prometheus.Labels{"uuid": gpuID}).Set(pct)

	default:
		log.Logger.Warnw("unsupported gpm metric id", "id", metricID)
	}
}

func init() {
	pkgmetrics.MustRegister(
		metricGPUSMOccupancyPercent,
		metricGPUIntUtilPercent,
		metricGPUAnyTensorUtilPercent,
		metricGPUDFMATensorUtilPercent,
		metricGPUHMMATensorUtilPercent,
		metricGPUIMMATensorUtilPercent,
		metricGPUFp64UtilPercent,
		metricGPUFp32UtilPercent,
		metricGPUFp16UtilPercent,
	)
}
