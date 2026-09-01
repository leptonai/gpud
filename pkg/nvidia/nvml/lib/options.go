package lib

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	nvlibdevice "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	nvinfo "github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	nvdl "github.com/NVIDIA/go-nvml/pkg/dl"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/leptonai/gpud/pkg/log"
)

const (
	// EnvNVMLLibraryPath explicitly selects the NVML shared library.
	//
	// This remains the only environment override for library selection.
	// The two driver roots below are deliberately NOT configurable via
	// environment variables: the gpud DaemonSet mounts them at these fixed,
	// well-known paths by default (gpud.mountHostRoot and
	// gpud.mountNVIDIADriverRoot), so an env var could only restate -- or
	// disagree with -- the actual mount layout without adding flexibility.
	EnvNVMLLibraryPath = "GPUD_NVML_LIBRARY_PATH"

	// defaultHostRoot is the well-known container path where the gpud
	// DaemonSet mounts the host root filesystem read-only, exposing a
	// pre-installed host driver's libraries under "<root>/usr/lib...".
	// It also exposes the GPU Operator's driver-ready validation contract,
	// which gpud uses to discover a non-default driver install directory.
	defaultHostRoot = "/host"

	// defaultDriverRoot is the well-known container path where the gpud
	// DaemonSet mounts the GPU Operator's driver installation tree.
	defaultDriverRoot = "/run/nvidia/driver"
)

type Op struct {
	nvmlLib nvml.Interface

	initReturn        *nvml.Return
	propertyExtractor nvinfo.PropertyExtractor
	devicesToReturn   []nvlibdevice.Device

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
	devGetRemappedRowsForAllDevs func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return)

	// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
	devGetCurrentClocksEventReasonsForAllDevs func() (uint64, nvml.Return)

	// devGetDevicesError is the error to return from Device().GetDevices().
	// Used for testing NVML device enumeration failure scenarios.
	devGetDevicesError error
}

type OpOption func(*Op)

func (op *Op) applyOpts(opts []OpOption) {
	for _, opt := range opts {
		opt(op)
	}
	if op.nvmlLib == nil {
		libraryPath := resolveNVMLLibraryPath()
		if libraryPath == "" {
			op.nvmlLib = nvml.New()
		} else {
			// The companions must be loaded BEFORE the first NVML call
			// (go-nvml dlopens libnvidia-ml lazily at nvml.New). See
			// preloadNVMLCompanionLibraries for why resolving the library
			// path alone is not sufficient inside a container.
			preloadNVMLCompanionLibraries(libraryPath)
			op.nvmlLib = nvml.New(nvml.WithLibraryPath(libraryPath))
		}
	}
}

// resolveNVMLLibraryPath returns an explicitly configured library first, then
// probes the well-known driver roots that the gpud DaemonSet mounts by
// default. It returns empty when no driver tree provides a library,
// preserving go-nvml's standard system-library lookup.
//
// Every probe is existence-based, so on machines without these mounts (e.g.
// bare-metal/systemd installations, where "/host" does not exist) resolution
// falls through to the system lookup exactly as before.
func resolveNVMLLibraryPath() string {
	if libraryPath := os.Getenv(EnvNVMLLibraryPath); libraryPath != "" {
		return libraryPath
	}
	return resolveFromDriverRoots(defaultHostRoot, defaultDriverRoot)
}

// resolveFromDriverRoots probes the host root and the GPU Operator driver
// root for a usable NVML library. It takes the roots as parameters (rather
// than reading the package constants) so tests can point the probes at
// temporary directories.
//
// The probe order mirrors the GPU Operator's own driver-validation order: a
// pre-installed host driver wins over the Operator-managed driver root when
// both are present. Loading the host's own libnvidia-ml guarantees the
// userspace library matches the loaded kernel module.
func resolveFromDriverRoots(hostRoot, driverRoot string) string {
	if libraryPath := probeNVMLLibrary(hostRoot); libraryPath != "" {
		return libraryPath
	}

	// The GPU Operator records its validated driver root in the
	// driver-ready contract; honor it to cover a non-default driver
	// install directory (spec.hostPaths.driverInstallDir). The contract
	// file lives on the host, so it is only readable through the host-root
	// mount.
	if libraryPath := probeDriverReadyContract(hostRoot); libraryPath != "" {
		return libraryPath
	}

	return probeNVMLLibrary(driverRoot)
}

// probeNVMLLibrary returns the first existing NVML shared library under the
// given root, covering the Debian/Ubuntu multiarch, RHEL lib64, and plain
// usr/lib layouts. It returns empty when the root has no NVML library (e.g.,
// the GPU Operator has not finished installing the driver yet).
func probeNVMLLibrary(root string) string {
	archLibraryDir := nvmlArchLibraryDir(runtime.GOARCH)
	candidates := []string{
		filepath.Join(root, "usr", "lib", archLibraryDir, "libnvidia-ml.so.1"),
		filepath.Join(root, "usr", "lib64", "libnvidia-ml.so.1"),
		filepath.Join(root, "usr", "lib", "libnvidia-ml.so.1"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// driverReadyContractPath is the GPU Operator's driver validation contract,
// relative to the host root. The Operator's driver-validation step records
// the selected NVIDIA_DRIVER_ROOT here after the driver validates, so it
// reflects a non-default driver install directory
// (spec.hostPaths.driverInstallDir).
const driverReadyContractPath = "run/nvidia/validations/driver-ready"

// probeDriverReadyContract reads the GPU Operator's driver-ready contract
// under the given host root and probes the NVIDIA_DRIVER_ROOT it selects,
// resolved under the same host root. It returns empty when the contract is
// absent, unparseable, names a non-absolute path, or selects the host driver
// ("/", already covered by the standard host-root probes). The contract's
// DRIVER_ROOT_CTR_PATH is the DRA container's own mount path and does not
// apply to gpud's mount layout.
func probeDriverReadyContract(hostRoot string) string {
	b, err := os.ReadFile(filepath.Join(hostRoot, driverReadyContractPath))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "NVIDIA_DRIVER_ROOT=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if value == "" || value == "/" || !filepath.IsAbs(value) {
			continue
		}
		if libraryPath := probeNVMLLibrary(filepath.Join(hostRoot, value)); libraryPath != "" {
			return libraryPath
		}
	}
	return ""
}

// nvmlCompanionLibraryGlobs are the shared libraries that a driver-root
// libnvidia-ml.so.1 needs at run time, expressed as globs against the
// directory that libnvidia-ml.so.1 was resolved from.
//
// WHY these families: libnvidia-ml.so.1 dlopens libcuda.so.1 BY SONAME at
// run time (confirmed by the SONAME strings embedded in the 580.x binary and
// by runtime A/B tests on AKS A100 and NSCALE B200 nodes): when libcuda.so.1
// cannot be found, every NVML query fails with NVML_ERROR_NOT_FOUND -- e.g.
// "failed to get driver version: Not Found" -- even though libnvidia-ml
// itself dlopened successfully. On 560+ driver trees, libcuda in turn loads
// libnvidia-gpucomp.so.<version> for compute-related NVML queries, and NVML
// uses libnvidia-cfg.so.1 for configuration queries. All three are therefore
// preloaded whenever they exist alongside the resolved libnvidia-ml.
//
// WHY an explicit allowlist instead of LD_LIBRARY_PATH=<driver lib dir>: the
// GPU Operator's driver tree bundles its OWN glibc in the same directory
// (usr/lib/x86_64-linux-gnu/libc.so.6). Putting the whole directory on
// LD_LIBRARY_PATH shadows the process glibc and crashes the gpud binary
// itself ("libc.so.6: undefined symbol: __tunable_is_initialized, version
// GLIBC_PRIVATE", observed on NSCALE driver 580.82.07). Preloading only the
// NVIDIA companions by absolute path never touches libc/libpthread/libm: the
// companions' own DT_NEEDED entries dedup against the glibc already loaded
// in the gpud process.
var nvmlCompanionLibraryGlobs = []string{
	"libcuda.so*",
	"libnvidia-cfg.so*",
	"libnvidia-gpucomp.so*",
}

// findNVMLCompanionLibraries returns, for each companion family, the single
// library file to preload from the directory of the resolved NVML library
// path. It returns nil for a bare SONAME (e.g., "libnvidia-ml.so.1" with no
// directory), because then resolution already defers to the system search
// path, which also provides the companions.
//
// Kept separate from preloadNVMLCompanionLibraries so unit tests can assert
// the selection without dlopening real driver libraries.
func findNVMLCompanionLibraries(libraryPath string) []string {
	dir := filepath.Dir(libraryPath)
	if dir == "." || dir == "" {
		return nil
	}

	var selected []string
	for _, pattern := range nvmlCompanionLibraryGlobs {
		// filepath.Glob returns sorted matches; a missing family is not an
		// error (existence-based, mirroring the driver-root probes: older
		// drivers ship no libnvidia-gpucomp, and that is fine).
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil || len(matches) == 0 {
			continue
		}

		// Prefer the SONAME symlink (e.g., "libcuda.so.1") over the dev
		// symlink ("libcuda.so") and the versioned file
		// ("libcuda.so.580.82.07"): the SONAME is the name libnvidia-ml
		// looks up, and dlopening the symlink still registers the target's
		// DT_SONAME, which is what makes the later by-SONAME lookup bind to
		// this exact copy. libnvidia-gpucomp ships only as the versioned
		// file, so "first sorted match" is the fallback.
		pick := matches[0]
		for _, m := range matches {
			if strings.HasSuffix(m, ".so.1") {
				pick = m
				break
			}
		}
		selected = append(selected, pick)
	}
	return selected
}

// preloadNVMLCompanionLibraries dlopens the NVML companion libraries from
// the directory of the resolved NVML library path, and intentionally never
// dlcloses them (they must stay loaded for the process lifetime, exactly
// like libnvidia-ml itself).
//
// WHY this exists: the driver-root probes above fix WHERE libnvidia-ml.so.1
// is found (a container that requests no GPU resources gets no toolkit
// injection, so the driver tree mounted at /host or /run/nvidia/driver is
// the only source), but an absolute-path dlopen of libnvidia-ml does NOT
// make that tree's companion libraries discoverable -- libnvidia-ml looks
// them up by bare SONAME through the standard search path, which does not
// include the driver root. Preloading each companion by absolute path
// registers its SONAME as already-loaded, so libnvidia-ml's later
// dlopen("libcuda.so.1") binds to the copy from the SAME driver tree. That
// keeps the whole NVML userspace stack version-consistent with each other
// and with the kernel module, which is precisely the guarantee the
// host-driver-first probe order is meant to provide.
//
// Without this, a no-injection BYOK container loaded libnvidia-ml from the
// driver root but every NVML call returned NOT_FOUND, and
// "gpud run --require-nvidia-driver" crash-looped nodes whose driver was
// perfectly healthy (verified on AKS A100 nodes with a pre-installed host
// driver via /host, and on NSCALE B200 nodes with the GPU Operator driver
// via /run/nvidia/driver).
func preloadNVMLCompanionLibraries(libraryPath string) {
	companions := findNVMLCompanionLibraries(libraryPath)
	if len(companions) == 0 {
		return
	}
	for _, companion := range companions {
		// Same flags go-nvml uses for libnvidia-ml itself
		// (dl.RTLD_LAZY|dl.RTLD_GLOBAL). A dlopen failure here is logged but
		// not fatal: companion loading must not change the existence-based
		// contract of the probes -- if the driver tree is genuinely broken,
		// the subsequent NVML calls surface that as before.
		if err := nvdl.New(companion, nvdl.RTLD_LAZY|nvdl.RTLD_GLOBAL).Open(); err != nil {
			log.Logger.Warnw("failed to preload NVML companion library", "library", companion, "error", err)
			continue
		}
		log.Logger.Infow("preloaded NVML companion library from driver tree", "library", companion)
	}
}

func nvmlArchLibraryDir(goarch string) string {
	switch goarch {
	case "arm64":
		return "aarch64-linux-gnu"
	case "ppc64le":
		return "powerpc64le-linux-gnu"
	default:
		return "x86_64-linux-gnu"
	}
}

// Specifies the NVML library instance.
// Otherwise, defaults to the NVML library instance returned by nvml.New().
func WithNVML(nvmlLib nvml.Interface) OpOption {
	return func(op *Op) {
		op.nvmlLib = nvmlLib
	}
}

// Specifies the return value of the NVML library's Init() function.
// Otherwise, defaults to the return value of the NVML library's Init() function.
func WithInitReturn(initReturn nvml.Return) OpOption {
	return func(op *Op) {
		op.initReturn = &initReturn
	}
}

// Specifies the property extractor for the NVML library.
func WithPropertyExtractor(propertyExtractor nvinfo.PropertyExtractor) OpOption {
	return func(op *Op) {
		op.propertyExtractor = propertyExtractor
	}
}

func WithDevice(dev nvlibdevice.Device) OpOption {
	return func(op *Op) {
		op.devicesToReturn = append(op.devicesToReturn, dev)
	}
}

// Specifies the function for all devices to get the remapped rows of the device.
// Otherwise, defaults to the function returned by device.GetRemappedRows().
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g055e7c34f7f15b6ae9aac1dabd60870d
func WithDeviceGetRemappedRowsForAllDevs(f func() (corrRows int, uncRows int, isPending bool, failureOccurred bool, ret nvml.Return)) OpOption {
	return func(op *Op) {
		op.devGetRemappedRowsForAllDevs = f
	}
}

// Specifies the function for all devices  to get the current clocks event reasons of the device.
// Otherwise, defaults to the function returned by device.GetCurrentClocksEventReasons().
// ref. https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html#group__nvmlDeviceQueries_1g7e505374454a0d4fc7339b6c885656d6
func WithDeviceGetCurrentClocksEventReasonsForAllDevs(f func() (uint64, nvml.Return)) OpOption {
	return func(op *Op) {
		op.devGetCurrentClocksEventReasonsForAllDevs = f
	}
}

// WithDeviceGetDevicesError specifies the error to return from Device().GetDevices().
// This is used for testing NVML device enumeration failure scenarios, such as when
// nvidia-smi shows "Unable to determine the device handle for GPU: Unknown Error".
func WithDeviceGetDevicesError(err error) OpOption {
	return func(op *Op) {
		op.devGetDevicesError = err
	}
}
