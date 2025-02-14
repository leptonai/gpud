package query

var (
	DefaultNVIDIALibraries = map[string][]string{
		// core library for NVML
		// typically symlinked to "libnvidia-ml.so.1" or "libnvidia-ml.so.535.183.06" (or other driver versions)
		// some latest drivers do not have this symlink, only "libnvidia-ml.so.1"
		"libnvidia-ml.so": {
			// core library for NVML
			// typically symlinked to "libnvidia-ml.so.565.57.01" (or other driver versions)
			// some latest drivers do not have this "libnvidia-ml.so" symlink, only "libnvidia-ml.so.1"
			"libnvidia-ml.so.1",
		},

		// core library for CUDA support
		// typically symlinked to "libcuda.so.1" or "libcuda.so.535.183.06"
		"libcuda.so": {
			"libcuda.so.1",
		},
	}

	DefaultNVIDIALibrariesSearchDirs = []string{
		// ref. https://github.com/NVIDIA/nvidia-container-toolkit/blob/main/internal/lookup/library.go#L33-L62
		"/",
		"/usr/lib64",
		"/usr/lib/x86_64-linux-gnu",
		"/usr/lib/aarch64-linux-gnu",
		"/usr/lib/x86_64-linux-gnu/nvidia/current",
		"/usr/lib/aarch64-linux-gnu/nvidia/current",
		"/lib64",
		"/lib/x86_64-linux-gnu",
		"/lib/aarch64-linux-gnu",
		"/lib/x86_64-linux-gnu/nvidia/current",
		"/lib/aarch64-linux-gnu/nvidia/current",
	}
)
