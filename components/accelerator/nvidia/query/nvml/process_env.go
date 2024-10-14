package nvml

// ports "DCGM_FR_BAD_CUDA_ENV"; The environment has variables that hurt CUDA
// ref. https://github.com/NVIDIA/DCGM/blob/903d745504f50153be8293f8566346f9de3b3c93/nvvs/plugin_src/software/Software.cpp#L839-L876
var BAD_CUDA_ENV_KEYS = map[string]string{
	"NSIGHT_CUDA_DEBUGGER":              "Setting NSIGHT_CUDA_DEBUGGER=1 can degrade the performance of an application, since the debugger is made resident. See https://docs.nvidia.com/nsight-visual-studio-edition/3.2/Content/Attach_CUDA_to_Process.htm.",
	"CUDA_INJECTION32_PATH":             "Captures information about CUDA execution trace. See https://docs.nvidia.com/nsight-systems/2020.3/tracing/index.html.",
	"CUDA_INJECTION64_PATH":             "Captures information about CUDA execution trace. See https://docs.nvidia.com/nsight-systems/2020.3/tracing/index.html.",
	"CUDA_AUTO_BOOST":                   "Automatically selects the highest possible clock rate allowed by the thermal and power budget. Independent of the global default setting the autoboost behavior can be overridden by setting the environment variable CUDA_AUTO_BOOST. Set CUDA_AUTO_BOOST=0 to disable frequency throttling/boosting. You may run 'nvidia-smi --auto-boost-default=0' to disable autoboost by default. See https://developer.nvidia.com/blog/increase-performance-gpu-boost-k80-autoboost/.",
	"CUDA_ENABLE_COREDUMP_ON_EXCEPTION": "Enables GPU core dumps.",
	"CUDA_COREDUMP_FILE":                "Enables GPU core dumps.",
	"CUDA_DEVICE_WAITS_ON_EXCEPTION":    "CUDA kernel will pause when an exception occurs. This is only useful for debugging.",
	"CUDA_PROFILE":                      "Enables CUDA profiling.",
	"COMPUTE_PROFILE":                   "Enables compute profiling.",
	"OPENCL_PROFILE":                    "Enables OpenCL profiling.",
}
